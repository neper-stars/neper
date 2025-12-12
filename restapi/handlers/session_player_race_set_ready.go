package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewSessionPlayerRaceSetReadyHandler ...
func NewSessionPlayerRaceSetReadyHandler(log *zerolog.Logger, db *sqlx.DB) *SessionPlayerRaceSetReadyHandler {
	return &SessionPlayerRaceSetReadyHandler{db, log}
}

// SessionPlayerRaceSetReadyHandler handles setting the ready flag on a player's race for a session
type SessionPlayerRaceSetReadyHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *SessionPlayerRaceSetReadyHandler) handle(
	ctx context.Context, params operations.SessionPlayerRaceSetReadyParams, principal *models.Principal,
) (*models.SessionPlayerRace, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Find the session_player_race for this user in this session
	spr := models.Schema.SessionPlayerRaceDB.As("spr")

	q := database.SQ.Select().
		Columns(spr.FQColumns(true)...).
		From(spr.Sql()).
		Where(spr.SessionID.Eq(params.SessionID)).
		Where(spr.UserProfileID.Eq(principal.Subject))

	var sprDB models.SessionPlayerRaceDB
	if err := sqlH.Get(&sprDB, q); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSomethingNotFound("no race registered for this user in session: " + params.SessionID)
		}
		return nil, err
	}

	// Authorization check
	authorized, err := h.Authorize(params, principal, &sprDB)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	// Update the ready flag
	sprDB.Ready = params.Ready
	if err := sqlH.Update(&sprDB); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &sprDB.SessionPlayerRace, nil
}

// Handle handles the request
func (h *SessionPlayerRaceSetReadyHandler) Handle(
	params operations.SessionPlayerRaceSetReadyParams, principal *models.Principal,
) middleware.Responder {
	spr, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionPlayerRaceSetReadyForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return BadRequest(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionPlayerRaceSetReadyOK().WithPayload(spr)
}

// Authorize checks if the user is allowed to modify the ready status
func (h *SessionPlayerRaceSetReadyHandler) Authorize(
	params operations.SessionPlayerRaceSetReadyParams, principal *models.Principal, sprDB *models.SessionPlayerRaceDB,
) (bool, error) {
	// Global managers can modify any ready status
	if principal.IsGlobalManager {
		return true, nil
	}

	// Users can only modify their own ready status
	// since this comes from a request with the current user id, it should
	// never happen, but our role is to safeguard against an error in the other layers
	if sprDB.UserProfileID != principal.Subject {
		return false, nil
	}

	return true, nil
}
