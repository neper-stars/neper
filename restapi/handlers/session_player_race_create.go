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

	neper "github.com/neper-stars/neper/lib"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/lib/race"
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

	// Validate bot race ID if this is a bot player
	if params.SessionPlayerRace.IsBot {
		if !race.IsValidBotRaceID(params.SessionPlayerRace.RaceID) {
			return nil, errs.NewErrInvalidSomething("invalid bot race ID, must be 0-6")
		}
		if params.SessionPlayerRace.BotLevel == nil {
			return nil, errs.NewErrInvalidSomething("bot_level is required for bot players")
		}
	}

	// ** AUTHORIZATION **
	authorized, err := h.Authorize(sqlH, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}
	// ** AUTHORIZATION END **

	var sessionPlayerRaceDB models.SessionPlayerRaceDB
	providedID := params.SessionPlayerRace.ID

	// If ID is provided, this is an update
	if providedID != "" {
		var existingSPR models.SessionPlayerRaceDB
		if err := sqlH.GetByPKey(&existingSPR, providedID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, errs.NewErrSomethingNotFound("player not found with id: " + providedID)
			}
			return nil, err
		}

		// Verify the entry belongs to this session
		if existingSPR.SessionID != params.SessionID {
			return nil, errs.NewErrInvalidSomething("player does not belong to this session")
		}

		// Authorization for updates:
		// - Bots: only managers can update (already checked in Authorize)
		// - Humans: must be the owner
		if !existingSPR.IsBot && existingSPR.UserProfileID != principal.Subject {
			return nil, errs.ErrForbidden
		}

		// Cannot change bot status
		if existingSPR.IsBot != params.SessionPlayerRace.IsBot {
			return nil, errs.NewErrInvalidSomething("cannot change player type between bot and human")
		}

		// Cannot update while ready
		if existingSPR.Ready {
			return nil, errs.NewErrInvalidSomething("cannot update race while player is ready")
		}

		// Update existing entry - keep the same ID, player_order, and user_profile_id
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

		if err := tx.Commit(); err != nil {
			return nil, err
		}

		if h.notifyService != nil {
			_ = h.notifyService.PublishSessionPlayerRaceUpdate(sessionPlayerRaceDB.ID)
		}

		return &sessionPlayerRaceDB.SessionPlayerRace, nil
	}

	// No ID provided - create new entry
	uid, err := uuid.V4()
	if err != nil {
		return nil, err
	}
	sessionPlayerRaceDB.SessionPlayerRace = *params.SessionPlayerRace
	sessionPlayerRaceDB.ID = uid.String()
	sessionPlayerRaceDB.SessionID = params.SessionID

	// For bots, use the system user ID
	// For human players, use the principal's user ID
	if params.SessionPlayerRace.IsBot {
		sessionPlayerRaceDB.UserProfileID = neper.SystemUserID
	} else {
		sessionPlayerRaceDB.UserProfileID = principal.Subject

		// Enforce uniqueness for non-system users: only one entry per session per human player
		var existingCount int64
		if err := sqlH.Get(&existingCount, database.SQ.
			Select("COUNT(*)").
			From(models.SessionPlayerRaceDBTable).
			Where(sq.And{
				sq.Eq{models.SessionPlayerRaceDBSessionIDColumn: params.SessionID},
				sq.Eq{models.SessionPlayerRaceDBUserProfileIDColumn: principal.Subject},
			})); err != nil {
			return nil, err
		}
		if existingCount > 0 {
			return nil, errs.NewErrInvalidSomething("player already registered in this session")
		}
	}

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

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if h.notifyService != nil {
		_ = h.notifyService.PublishSessionPlayerRaceCreate(sessionPlayerRaceDB.ID)
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
	// Pending users cannot assign races to sessions
	if principal.IsPending {
		return false, nil
	}

	sessionID := params.SessionID
	isBot := params.SessionPlayerRace.IsBot

	// Check if user is a session manager
	manager, err := IsSessionManager(sqlH, principal.Subject, sessionID)
	if err != nil {
		return false, err
	}

	// Bot players can only be added by managers (session or global)
	if isBot {
		if principal.IsGlobalManager || manager {
			return true, nil
		}
		return false, nil
	}

	// For human players, global managers can do anything
	if principal.IsGlobalManager {
		return true, nil
	}

	// Check if user is a session member
	member, err := IsSessionMember(sqlH, principal.Subject, sessionID)
	if err != nil {
		return false, err
	}

	if !member && !manager {
		return false, nil
	}

	// For human players, verify the race belongs to the user
	raceID := params.SessionPlayerRace.RaceID
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
