package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/m4rw3r/uuid"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	"database/sql"

	sq "github.com/Masterminds/squirrel"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewSessionPlayerRaceCreateHandler ...
func NewSessionPlayerRaceCreateHandler(log *zerolog.Logger, db *sqlx.DB) *SessionPlayerRaceCreateHandler {
	return &SessionPlayerRaceCreateHandler{db, log}
}

// SessionPlayerRaceCreateHandler handles /Races
type SessionPlayerRaceCreateHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
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

	var sessionPlayerRaceDB models.SessionPlayerRaceDB
	uid, err := uuid.V4()
	if err != nil {
		return nil, err
	}
	sessionPlayerRaceDB.SessionPlayerRace = *params.SessionPlayerRace
	sessionPlayerRaceDB.ID = uid.String()
	sessionPlayerRaceDB.SessionID = params.SessionID
	sessionPlayerRaceDB.UserProfileID = principal.Subject

	_, err = sqlH.Insert(&sessionPlayerRaceDB)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
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
	if params.SessionID != params.SessionPlayerRace.SessionID {
		// this kind of trick is a nono
		return false, nil
	}

	sessionID := params.SessionID
	raceID := params.SessionPlayerRace.RaceID

	var sessionRel models.UserProfileSessionRelDB
	filter := sq.And{
		sq.Eq{models.UserProfileSessionRelDBSessionIDColumn: sessionID},
		sq.Eq{models.UserProfileSessionRelDBUserProfileIDColumn: principal.Subject},
	}
	if err := sqlH.GetWhere(&sessionRel, filter); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// session has no relation for the given user... --> refuse without error
			return false, nil
		}
		return false, err
	}

	// TODO: we could optimize to load only the userID and not the whole dataset
	var raceDB models.RaceDB
	filter = sq.And{
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
