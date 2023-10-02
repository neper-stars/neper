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

// NewRaceReadHandler ...
func NewRaceReadHandler(db *sqlx.DB) *RaceReadHandler {
	return &RaceReadHandler{db}
}

// RaceReadHandler handles /user_profiles
type RaceReadHandler struct {
	db *sqlx.DB
}

func (h *RaceReadHandler) handle(
	ctx context.Context, params operations.RaceReadParams, principal *models.Principal,
) (*models.Race, error) {
	authorized, err := h.Authorize(ctx, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	raceID := params.RaceID
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	var raceDB models.RaceDB
	if err := sqlH.GetByPKey(&raceDB, raceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrRaceNotFound("race not found with ID: " + raceID)
		}
		return nil, err
	}
	return &raceDB.Race, nil
}

// Handle handles the request
func (h *RaceReadHandler) Handle(
	params operations.RaceReadParams, principal *models.Principal,
) middleware.Responder {
	race, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewRaceReadForbidden().WithPayload(&models.Error{
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
	return operations.NewRaceReadOK().WithPayload(race)
}

func (h *RaceReadHandler) Authorize(
	ctx context.Context, params operations.RaceReadParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	if params.UserProfileID == principal.Subject {
		return true, nil
	}
	return false, nil
}
