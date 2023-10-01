package handlers

import (
	"context"
	"errors"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/m4rw3r/uuid"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
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
	ctx context.Context, userProfile *models.UserProfile,
) (*models.UserProfile, error) {
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

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &userProfileDB.UserProfile, nil
}

// Handle handles the request
func (h *UserProfileCreateHandler) Handle(
	params operations.UserProfileCreateParams, principal *models.Principal,
) middleware.Responder {
	userProfile, err := h.handle(params.HTTPRequest.Context(), params.UserProfile)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewUserProfileCreateCreated().WithPayload(userProfile)
}
