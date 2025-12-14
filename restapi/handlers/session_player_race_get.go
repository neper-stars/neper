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

// NewSessionPlayerRaceGetHandler creates a new handler for getting the player's race in a session
func NewSessionPlayerRaceGetHandler(db *sqlx.DB) *SessionPlayerRaceGetHandler {
	return &SessionPlayerRaceGetHandler{db: db}
}

// SessionPlayerRaceGetHandler handles GET /sessions/{session_id}/player_race
type SessionPlayerRaceGetHandler struct {
	db *sqlx.DB
}

func (h *SessionPlayerRaceGetHandler) handle(
	ctx context.Context, params operations.SessionPlayerRaceGetParams, principal *models.Principal,
) (*models.Race, error) {
	sessionID := params.SessionID

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Get the player's session player race mapping
	var sessionPlayerRace models.SessionPlayerRaceDB
	sprQuery := sessionPlayerRaceQuery(principal.Subject, sessionID)
	if err := sqlH.Get(&sessionPlayerRace, sprQuery); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSomethingNotFound("no race set for this session")
		}
		return nil, err
	}

	// Get the actual race using the race_id from the session player race
	var raceDB models.RaceDB
	if err := sqlH.GetByPKey(&raceDB, sessionPlayerRace.RaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrRaceNotFound("race not found with ID: " + sessionPlayerRace.RaceID)
		}
		return nil, err
	}

	return &raceDB.Race, nil
}

// Handle handles the request
func (h *SessionPlayerRaceGetHandler) Handle(
	params operations.SessionPlayerRaceGetParams, principal *models.Principal,
) middleware.Responder {
	race, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionPlayerRaceGetForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return operations.NewSessionPlayerRaceGetNotFound().WithPayload(models.FromError(err))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionPlayerRaceGetOK().WithPayload(race)
}
