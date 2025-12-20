package handlers

import (
	"context"
	"database/sql"
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

// NewPendingRegistrationRejectHandler creates a new handler
func NewPendingRegistrationRejectHandler(log *zerolog.Logger, db *sqlx.DB, notifyService *notify.Service) *PendingRegistrationRejectHandler {
	return &PendingRegistrationRejectHandler{db: db, log: log, notifyService: notifyService}
}

// PendingRegistrationRejectHandler handles DELETE /pending_registrations/{user_profile_id}/reject
type PendingRegistrationRejectHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	notifyService *notify.Service
}

func (h *PendingRegistrationRejectHandler) handle(
	ctx context.Context, params operations.PendingRegistrationRejectParams, principal *models.Principal,
) error {
	authorized, err := h.Authorize(ctx, params, principal)
	if err != nil {
		return err
	}
	if !authorized {
		return errs.ErrForbidden
	}

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Get the pending user profile to verify it exists and is pending
	var userDB models.UserProfileDB
	if err := sqlH.GetByPKey(&userDB, params.UserProfileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewErrSomethingNotFound("user profile not found")
		}
		return err
	}

	// Check if it's actually pending
	if !userDB.Pending {
		return errs.NewErrInvalidSomething("user profile is not pending approval")
	}

	// Delete the pending user profile
	_, err = sqlH.Exec(database.SQ.
		Delete(models.UserProfileDBTable).
		Where(sq.Eq{models.UserProfileDBIDColumn: params.UserProfileID}))
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Info().
		Str("user_id", params.UserProfileID).
		Str("nickname", userDB.Nickname).
		Str("rejected_by", principal.Subject).
		Msg("pending registration rejected")

	// Notify global managers about the rejection
	if h.notifyService != nil {
		if err := h.notifyService.PublishPendingRegistrationReject(params.UserProfileID); err != nil {
			log.Err(err).Msg("failed to publish pending registration rejection notification")
		}
	}

	return nil
}

// Handle handles the request
func (h *PendingRegistrationRejectHandler) Handle(
	params operations.PendingRegistrationRejectParams, principal *models.Principal,
) middleware.Responder {
	err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewPendingRegistrationRejectForbidden().WithPayload(&models.Error{
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
	return operations.NewPendingRegistrationRejectNoContent()
}

// Authorize checks if the principal is allowed to reject pending registrations
func (h *PendingRegistrationRejectHandler) Authorize(
	ctx context.Context, params operations.PendingRegistrationRejectParams, principal *models.Principal,
) (bool, error) {
	// Only global managers can reject registrations
	if principal.IsGlobalManager {
		return true, nil
	}
	return false, nil
}
