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
	"github.com/neper-stars/neper/lib/serial"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewPendingRegistrationApproveHandler creates a new handler
func NewPendingRegistrationApproveHandler(log *zerolog.Logger, db *sqlx.DB, notifyService *notify.Service) *PendingRegistrationApproveHandler {
	return &PendingRegistrationApproveHandler{db: db, log: log, notifyService: notifyService}
}

// PendingRegistrationApproveHandler handles POST /pending_registrations/{user_profile_id}/approve
type PendingRegistrationApproveHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	notifyService *notify.Service
}

func (h *PendingRegistrationApproveHandler) handle(
	ctx context.Context, params operations.PendingRegistrationApproveParams, principal *models.Principal,
) (*models.ApikeyReset, error) {
	authorized, err := h.Authorize(ctx, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Get the pending user profile
	var userDB models.UserProfileDB
	if err := sqlH.GetByPKey(&userDB, params.UserProfileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSomethingNotFound("user profile not found")
		}
		return nil, err
	}

	// Check if it's actually pending
	if userDB.State != models.UserProfileStatePending {
		return nil, errs.NewErrInvalidSomething("user profile is not pending approval")
	}

	// Update the user profile: set state=active to grant full access
	// (API key was already set at registration)
	_, err = sqlH.Exec(database.SQ.
		Update(models.UserProfileDBTable).
		Set(models.UserProfileDBStateColumn, models.UserProfileStateActive).
		Where(sq.Eq{models.UserProfileDBIDColumn: params.UserProfileID}))
	if err != nil {
		return nil, err
	}

	// Try to assign a serial key (non-fatal if it fails)
	serialKey, err := serial.AssignKeyToUserTx(ctx, tx.Tx, params.UserProfileID)
	if err != nil {
		log.Warn().Err(err).Str("user_id", params.UserProfileID).Msg("failed to assign serial key during approval")
	} else {
		log.Info().Str("user_id", params.UserProfileID).Str("serial_key", serialKey).Msg("assigned serial key during approval")
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	log.Info().
		Str("user_id", params.UserProfileID).
		Str("nickname", userDB.Nickname).
		Str("approved_by", principal.Subject).
		Msg("pending registration approved")

	// Notify global managers (to update pending list) and the user (to know they've been approved)
	if h.notifyService != nil {
		if err := h.notifyService.PublishPendingRegistrationApprove(params.UserProfileID, userDB.Nickname); err != nil {
			log.Err(err).Msg("failed to publish pending registration approval notification")
		}
	}

	return &models.ApikeyReset{
		UserID: params.UserProfileID,
		Apikey: userDB.APIKey,
	}, nil
}

// Handle handles the request
func (h *PendingRegistrationApproveHandler) Handle(
	params operations.PendingRegistrationApproveParams, principal *models.Principal,
) middleware.Responder {
	result, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewPendingRegistrationApproveForbidden().WithPayload(&models.Error{
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
	return operations.NewPendingRegistrationApproveOK().WithPayload(result)
}

// Authorize checks if the principal is allowed to approve pending registrations
func (h *PendingRegistrationApproveHandler) Authorize(
	ctx context.Context, params operations.PendingRegistrationApproveParams, principal *models.Principal,
) (bool, error) {
	// Only global managers can approve registrations
	if principal.IsGlobalManager {
		return true, nil
	}
	return false, nil
}
