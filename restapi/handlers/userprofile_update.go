package handlers

import (
	"context"
	"errors"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewUserProfileUpdateHandler ...
func NewUserProfileUpdateHandler(db *sqlx.DB) *UserProfileUpdateHandler {
	return &UserProfileUpdateHandler{db}
}

// UserProfileUpdateHandler handles /circles
type UserProfileUpdateHandler struct {
	db *sqlx.DB
}

func (h *UserProfileUpdateHandler) handle(
	ctx context.Context, userProfile *models.UserProfile, principal *models.Principal,
) (*models.UserProfile, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	var userProfileDB models.UserProfileDB

	userProfileDB.UserProfile = *userProfile

	// allowed to change the nickname... NOT your manager level :)
	updateColumnList := []string{
		models.UserProfileDBNicknameColumn,
	}

	// only a global manager can set another user as manager
	// or change some specific fields
	if principal.IsGlobalManager {
		updateColumnList = append(
			updateColumnList,
			models.UserProfileDBIsManagerColumn,
			models.UserProfileDBIsActiveColumn,
			models.UserProfileDBEmailColumn,
		)
	}

	if err := sqlH.UpdateColumns(&userProfileDB, updateColumnList...); err != nil {
		return userProfile, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &userProfileDB.UserProfile, nil
}

// Handle handles the request
func (h *UserProfileUpdateHandler) Handle(
	params operations.UserProfileUpdateParams, principal *models.Principal,
) middleware.Responder {
	circle, err := h.handle(params.HTTPRequest.Context(), params.UserProfile, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewUserProfileUpdateOK().WithPayload(circle)
}
