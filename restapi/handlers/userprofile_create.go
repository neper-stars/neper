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

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/serial"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewUserProfileCreateHandler ...
func NewUserProfileCreateHandler(db *sqlx.DB) *UserProfileCreateHandler {
	return &UserProfileCreateHandler{db}
}

// UserProfileCreateHandler handles /sessions
type UserProfileCreateHandler struct {
	db *sqlx.DB
}

func (h *UserProfileCreateHandler) handle(
	ctx context.Context, params operations.UserProfileCreateParams, principal *models.Principal,
) (*models.UserProfile, error) {
	authorized, err := h.Authorize(ctx, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	userProfile := params.UserProfile

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	var userProfileDB models.UserProfileDB
	sessionUID, err := uuid.V4()
	if err != nil {
		return nil, err
	}
	// force our ID not the one from the client
	userProfile.ID = sessionUID.String()

	userProfileDB.UserProfile = *userProfile
	_, err = sqlH.Insert(&userProfileDB)
	if err != nil {
		return userProfile, err
	}

	// Assign a serial key to the new user within the same transaction
	// This is non-fatal - if it fails, the user can still be created
	serialKey, err := serial.AssignKeyToUserTx(ctx, tx.Tx, userProfile.ID)
	if err != nil {
		log.Warn().Err(err).Str("user_id", userProfile.ID).Msg("failed to assign serial key to new user")
	} else {
		log.Info().Str("user_id", userProfile.ID).Str("serial_key", serialKey).Msg("assigned serial key to new user")
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &userProfileDB.UserProfile, nil
}

// Handle handles the request
func (h *UserProfileCreateHandler) Handle(
	params operations.UserProfileCreateParams, principal *models.Principal,
) middleware.Responder {
	userProfile, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewUserProfileCreateForbidden().WithPayload(&models.Error{
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
	return operations.NewUserProfileCreateCreated().WithPayload(userProfile)
}

func (h *UserProfileCreateHandler) Authorize(
	ctx context.Context, params operations.UserProfileCreateParams, principal *models.Principal,
) (bool, error) {
	// only global manager can create profiles
	if principal.IsGlobalManager {
		return true, nil
	}
	return false, nil
}
