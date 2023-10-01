package handlers

import (
	"context"
	"errors"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/m4rw3r/uuid"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/lib"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewRaceCreateHandler ...
func NewRaceCreateHandler(log *zerolog.Logger, db *sqlx.DB) *RaceCreateHandler {
	return &RaceCreateHandler{db, log}
}

// RaceCreateHandler handles /Races
type RaceCreateHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *RaceCreateHandler) handle(
	ctx context.Context, inputRace *models.Race, principal *models.Principal,
) (*models.Race, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	var raceDB models.RaceDB
	raceUID, err := uuid.V4()
	if err != nil {
		return nil, err
	}

	race, err := lib.NewRace(inputRace.Data)
	if err != nil {
		h.log.Err(err).Msg("failed to parse race data from input")
		return nil, err
	}

	// force our ID not the one from the client
	race.ID = raceUID.String()
	// user ID is always the one from the principal
	race.UserID = principal.Subject

	raceDB.Race = *race
	_, err = sqlH.Insert(&raceDB)
	if err != nil {
		return race, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &raceDB.Race, nil
}

// Handle handles the request
func (h *RaceCreateHandler) Handle(
	params operations.RaceCreateParams, principal *models.Principal,
) middleware.Responder {
	race, err := h.handle(params.HTTPRequest.Context(), params.Race, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewRaceCreateCreated().WithPayload(race)
}
