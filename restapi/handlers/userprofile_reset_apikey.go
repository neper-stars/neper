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
	ctx context.Context, params operations.UserProfileResetApikeyParams, principal *models.Principal,
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

	// Check if user exists
	var userProfileDB models.UserProfileDB
	if err := sqlH.GetByPKey(&userProfileDB, params.UserProfileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrUserProfileNotFound("user_profile not found with ID: " + params.UserProfileID)
		}
		return nil, err
	}

	// Generate a random API key
	apiKeyHex, err := auth.GenerateAPIKey()
	if err != nil {
		return nil, err
	}

	// Update the API key in the database
	_, err = sqlH.Exec(database.SQ.
		Update(models.UserProfileDBTable).
		Set(models.UserProfileDBAPIKeyColumn, apiKeyHex).
		Where(sq.Eq{models.UserProfileDBIDColumn: params.UserProfileID}))
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &models.ApikeyReset{
		UserID: params.UserProfileID,
		Apikey: apiKeyHex,
	}, nil
}

var forbiddenMsg = "forbidden"

// Handle handles the request
func (h *UserProfileResetApikeyHandler) Handle(
	params operations.UserProfileResetApikeyParams, principal *models.Principal,
) middleware.Responder {
	result, err := h.handle(params.HTTPRequest.Context(), params, principal)
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

// Authorize checks if the principal is allowed to reset the API key
func (h *UserProfileResetApikeyHandler) Authorize(
	ctx context.Context, params operations.UserProfileResetApikeyParams, principal *models.Principal,
) (bool, error) {
	// User can reset their own key, or global manager can reset any key
	if principal.Subject == params.UserProfileID || principal.IsGlobalManager {
		return true, nil
	}
	return false, nil
}
