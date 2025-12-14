package handlers

import (
	"context"
	"errors"
	"net/http"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewRaceDeleteHandler ...
func NewRaceDeleteHandler(db *sqlx.DB, notifyService *notify.Service) *RaceDeleteHandler {
	return &RaceDeleteHandler{db: db, notifyService: notifyService}
}

// RaceDeleteHandler handles race deletion
type RaceDeleteHandler struct {
	db            *sqlx.DB
	notifyService *notify.Service
}

func (h *RaceDeleteHandler) handle(
	ctx context.Context, params operations.RaceDeleteParams, principal *models.Principal,
) error {
	authorized, err := h.Authorize(ctx, params, principal)
	if err != nil {
		return err
	}
	if !authorized {
		return errs.ErrForbidden
	}

	raceID := params.RaceID
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Check if race is used in any session
	countQuery := database.SQ.Select("COUNT(*)").
		From(models.SessionPlayerRaceDBTable).
		Where(sq.Eq{models.SessionPlayerRaceDBRaceIDColumn: raceID})

	var count int
	if err := sqlH.Get(&count, countQuery); err != nil {
		return err
	}
	if count > 0 {
		return errs.NewErrRaceInUse("race is in use in a session and cannot be deleted")
	}

	// Delete the race
	query := database.SQ.Delete(models.RaceDBTable).
		Where(sq.Eq{models.RaceDBIDColumn: raceID})

	result, err := sqlH.Exec(query)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errs.NewErrRaceNotFound("race not found with ID: " + raceID)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Publish notification after successful commit
	if h.notifyService != nil {
		_ = h.notifyService.PublishRaceDelete(raceID)
	}

	return nil
}

// Handle handles the request
func (h *RaceDeleteHandler) Handle(
	params operations.RaceDeleteParams, principal *models.Principal,
) middleware.Responder {
	err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewRaceDeleteForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrConflict):
			return Conflict(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewRaceDeleteNoContent()
}

func (h *RaceDeleteHandler) Authorize(
	ctx context.Context, params operations.RaceDeleteParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	if params.UserProfileID == principal.Subject {
		return true, nil
	}
	return false, nil
}
