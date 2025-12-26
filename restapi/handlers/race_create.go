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
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewRaceCreateHandler ...
func NewRaceCreateHandler(log *zerolog.Logger, db *sqlx.DB, runner *stars.Runner, notifyService *notify.Service) *RaceCreateHandler {
	return &RaceCreateHandler{db: db, log: log, runner: runner, notifyService: notifyService}
}

// RaceCreateHandler handles /Races
type RaceCreateHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	runner        *stars.Runner
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

	opts := neper.RaceFileOptions{
		StripPassword: h.runner != nil && h.runner.StripRacePasswords(),
		FixCorrupted:  h.runner != nil && h.runner.FixRaceFiles(),
	}
	race, analysis, err := neper.RaceFromString(inputRace.Data, opts)
	if err != nil {
		h.log.Err(err).Msg("failed to parse race data from input")
		return nil, errs.NewErrInvalidRace("failed to parse race file:" + err.Error())
	}

	// Log analysis results
	if analysis != nil {
		if analysis.NeedsRepair {
			if analysis.WasRepaired {
				h.log.Info().Str("user", principal.Subject).Msg("corrupted race file was automatically repaired")
			} else if analysis.RepairError != "" {
				h.log.Warn().Str("user", principal.Subject).Str("error", analysis.RepairError).Msg("race file is corrupted and repair failed")
			} else {
				h.log.Warn().Str("user", principal.Subject).Msg("race file is corrupted but automatic repair is disabled")
			}
		}
		if analysis.HasPassword {
			if analysis.PasswordStripped {
				h.log.Debug().Str("user", principal.Subject).Msg("password was stripped from race file")
			} else {
				h.log.Debug().Str("user", principal.Subject).Msg("race file has a password")
			}
		}
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
