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

	"github.com/neper-stars/neper/auth"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewUserProfileResetApikeyHandler creates a new handler
func NewUserProfileResetApikeyHandler(db *sqlx.DB) *UserProfileResetApikeyHandler {
	return &UserProfileResetApikeyHandler{db}
}

// UserProfileResetApikeyHandler handles reset API key requests
type UserProfileResetApikeyHandler struct {
	db *sqlx.DB
}

func (h *UserProfileResetApikeyHandler) handle(
	ctx context.Context, profileID string, principal *models.Principal,
) (*models.ApikeyReset, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Check if user exists
	var userProfileDB models.UserProfileDB
	if err := sqlH.GetByPKey(&userProfileDB, profileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrUserProfileNotFound("user_profile not found with ID: " + profileID)
		}
		return nil, err
	}

	// Check authorization: user can reset their own key, or manager can reset any key
	if principal.Subject != profileID && !principal.IsGlobalManager {
		return nil, errs.ErrForbidden
	}

	// Generate a random API key
	apiKeyHex, err := auth.GenerateAPIKey()
	if err != nil {
		return nil, err
	}

	// Update the API key in the database
	_, err = tx.ExecContext(ctx, `UPDATE user_profile SET apikey = $1 WHERE id = $2`, apiKeyHex, profileID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &models.ApikeyReset{
		UserID: profileID,
		Apikey: apiKeyHex,
	}, nil
}

var forbiddenMsg = "forbidden"

// Handle handles the request
func (h *UserProfileResetApikeyHandler) Handle(
	params operations.UserProfileResetApikeyParams, principal *models.Principal,
) middleware.Responder {
	result, err := h.handle(params.HTTPRequest.Context(), params.UserProfileID, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewUserProfileResetApikeyForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &forbiddenMsg,
			})
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewUserProfileResetApikeyOK().WithPayload(result)
}
