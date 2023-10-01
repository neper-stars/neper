package handlers

import (
	"context"
	"database/sql"
	"errors"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewUserProfileReadHandler ...
func NewUserProfileReadHandler(db *sqlx.DB) *UserProfileReadHandler {
	return &UserProfileReadHandler{db}
}

// UserProfileReadHandler handles /user_profiles
type UserProfileReadHandler struct {
	db *sqlx.DB
}

func (h *UserProfileReadHandler) handle(
	ctx context.Context, profileID string,
) (*models.UserProfile, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	var userProfileDB models.UserProfileDB
	if err := sqlH.GetByPKey(&userProfileDB, profileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrUserProfileNotFound("user_profile not found with ID: " + profileID)
		}
		return nil, err
	}

	return &userProfileDB.UserProfile, nil
}

// Handle handles the request
func (h *UserProfileReadHandler) Handle(
	params operations.UserProfileReadParams, principal *models.Principal,
) middleware.Responder {
	profile, err := h.handle(params.HTTPRequest.Context(), params.UserProfileID)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewUserProfileReadOK().WithPayload(profile)
}
