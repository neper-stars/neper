package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/m4rw3r/uuid"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewSessionPlayerRaceCreateHandler ...
func NewSessionPlayerRaceCreateHandler(log *zerolog.Logger, db *sqlx.DB, notifyService *notify.Service) *SessionPlayerRaceCreateHandler {
	return &SessionPlayerRaceCreateHandler{db: db, log: log, notifyService: notifyService}
}

// SessionPlayerRaceCreateHandler handles /Races
type SessionPlayerRaceCreateHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	notifyService *notify.Service
}

func (h *SessionPlayerRaceCreateHandler) handle(
	ctx context.Context, params operations.SessionPlayerRaceCreateParams, principal *models.Principal,
) (*models.SessionPlayerRace, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// ** AUTHORIZATION **
	authorized, err := h.Authorize(sqlH, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}
	// ** AUTHORIZATION END **

	// Check if a session_player_race already exists for this user in this session
	var existingSPR models.SessionPlayerRaceDB
	existingQuery := sessionPlayerRaceQuery(principal.Subject, params.SessionID)
	existingErr := sqlH.Get(&existingSPR, existingQuery)

	isUpdate := existingErr == nil
	if existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows) {
		return nil, existingErr
	}

	// If updating, check that the player is not ready
	if isUpdate && existingSPR.Ready {
		return nil, errs.NewErrInvalidSomething("cannot update race while player is ready")
	}

	var sessionPlayerRaceDB models.SessionPlayerRaceDB

	if isUpdate {
		// Update existing entry - keep the same ID and player_order
		sessionPlayerRaceDB = existingSPR
		sessionPlayerRaceDB.RaceID = params.SessionPlayerRace.RaceID
		sessionPlayerRaceDB.BotLevel = params.SessionPlayerRace.BotLevel

		_, err = sqlH.Exec(database.SQ.
			Update(models.SessionPlayerRaceDBTable).
			Set(models.SessionPlayerRaceDBRaceIDColumn, sessionPlayerRaceDB.RaceID).
			Set(models.SessionPlayerRaceDBBotLevelColumn, sessionPlayerRaceDB.BotLevel).
			Where(sq.Eq{models.SessionPlayerRaceDBIDColumn: sessionPlayerRaceDB.ID}))
		if err != nil {
			return nil, err
		}
	} else {
		// Create new entry
		uid, err := uuid.V4()
		if err != nil {
			return nil, err
		}
		sessionPlayerRaceDB.SessionPlayerRace = *params.SessionPlayerRace
		sessionPlayerRaceDB.ID = uid.String()
		sessionPlayerRaceDB.SessionID = params.SessionID
		sessionPlayerRaceDB.UserProfileID = principal.Subject

		// Automatically assign the next available player_order
		nextOrder, err := getNextPlayerOrder(sqlH, params.SessionID)
		if err != nil {
			return nil, err
		}
		sessionPlayerRaceDB.PlayerOrder = nextOrder

		_, err = sqlH.Insert(&sessionPlayerRaceDB)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Publish notification after successful commit
	if h.notifyService != nil {
		if isUpdate {
			_ = h.notifyService.PublishSessionPlayerRaceUpdate(sessionPlayerRaceDB.ID)
		} else {
			_ = h.notifyService.PublishSessionPlayerRaceCreate(sessionPlayerRaceDB.ID)
		}
	}

	return &sessionPlayerRaceDB.SessionPlayerRace, nil
}

// Handle handles the request
func (h *SessionPlayerRaceCreateHandler) Handle(
	params operations.SessionPlayerRaceCreateParams, principal *models.Principal,
) middleware.Responder {
	spr, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionPlayerRaceCreateForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrInvalid):
			return BadRequest(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionPlayerRaceCreateOK().WithPayload(spr)
}

func (h *SessionPlayerRaceCreateHandler) Authorize(
	sqlH database.SQLHelper, params operations.SessionPlayerRaceCreateParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}

	sessionID := params.SessionID
	raceID := params.SessionPlayerRace.RaceID

	member, err := IsSessionMember(sqlH, principal.Subject, sessionID)
	if err != nil {
		return false, err
	}
	manager, err := IsSessionManager(sqlH, principal.Subject, sessionID)
	if err != nil {
		return false, err
	}

	if !member && !manager {
		return false, nil
	}

	// TODO: we could optimize to load only the userID and not the whole dataset
	var raceDB models.RaceDB
	filter := sq.And{
		sq.Eq{models.RaceDBIDColumn: raceID},
		sq.Eq{models.RaceDBUserIDColumn: principal.Subject},
	}
	if err := sqlH.GetWhere(&raceDB, filter); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// race is not owned by the user... --> refuse without error
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// getNextPlayerOrder returns the next available player_order for a session.
// Player orders are 0-indexed (0-15 for up to 16 players).
func getNextPlayerOrder(sqlH database.SQLHelper, sessionID string) (int64, error) {
	// Count existing players in the session
	var count int64
	countQuery := database.SQ.Select("COUNT(*)").
		From(models.SessionPlayerRaceDBTable).
		Where(sq.Eq{models.SessionPlayerRaceDBSessionIDColumn: sessionID})

	if err := sqlH.Get(&count, countQuery); err != nil {
		return 0, err
	}

	// The next player order is simply the count of existing players
	// (0-indexed: if there are 0 players, next is 0; if there is 1 player, next is 1, etc.)
	return count, nil
}
