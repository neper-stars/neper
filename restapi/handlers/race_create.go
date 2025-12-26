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

	neper "github.com/neper-stars/neper/lib"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/lib/racefiles"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewRaceCreateHandler ...
func NewRaceCreateHandler(log *zerolog.Logger, db *sqlx.DB, raceProcessor *racefiles.Processor, notifyService *notify.Service) *RaceCreateHandler {
	return &RaceCreateHandler{db: db, log: log, raceProcessor: raceProcessor, notifyService: notifyService}
}

// RaceCreateHandler handles /Races
type RaceCreateHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	raceProcessor *racefiles.Processor
	notifyService *notify.Service
}

func (h *RaceCreateHandler) handle(
	ctx context.Context, params operations.RaceCreateParams, principal *models.Principal,
) (*models.Race, error) {
	authorized, err := h.Authorize(ctx, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}
	inputRace := params.Race

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

	var opts neper.RaceFileOptions
	if h.raceProcessor != nil {
		opts = h.raceProcessor.Options()
	}
	race, analysis, err := neper.RaceFromString(inputRace.Data, opts)
	if err != nil {
		h.log.Err(err).Msg("failed to parse race data from input")
		return nil, errs.NewErrInvalidRace("failed to parse race file:" + err.Error())
	}

	// Log analysis results
	racefiles.LogAnalysis(h.log, analysis, "user:"+principal.Subject)

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

	// Publish notification after successful commit
	if h.notifyService != nil {
		_ = h.notifyService.PublishRaceCreate(race.ID)
	}

	return &raceDB.Race, nil
}

// Handle handles the request
func (h *RaceCreateHandler) Handle(
	params operations.RaceCreateParams, principal *models.Principal,
) middleware.Responder {
	race, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewRaceCreateForbidden().WithPayload(&models.Error{
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
	return operations.NewRaceCreateCreated().WithPayload(race)
}

func (h *RaceCreateHandler) Authorize(
	ctx context.Context, params operations.RaceCreateParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	if params.UserProfileID == principal.Subject {
		return true, nil
	}
	return false, nil
}
