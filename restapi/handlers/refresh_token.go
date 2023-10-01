package handlers

import (
	"context"
	"errors"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewRefreshTokenHandler ...
func NewRefreshTokenHandler(log *zerolog.Logger, db *sqlx.DB, auth *auth.Auth) *RefreshToken {
	return &RefreshToken{log, db, auth}
}

// RefreshToken handles /auth/renew_token
type RefreshToken struct {
	log  *zerolog.Logger
	db   *sqlx.DB
	auth *auth.Auth
}

func (h *RefreshToken) handle(
	ctx context.Context, principal *models.Principal,
) (string, error) {
	// TODO Query the db and check it the profile is still valid (ie has an api key set),
	// is still connected to the account, and the account is still active

	valid, err := VerifUserIsValid(ctx, h.db, h.log, principal)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", auth.ErrInvalidCredentials
	}

	return h.auth.MakeToken(*principal)
}

// Handle handles the request
func (h *RefreshToken) Handle(
	params operations.RefreshTokenParams, principal *models.Principal,
) middleware.Responder {
	token, err := h.handle(params.HTTPRequest.Context(), principal)
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		// invalid creds
		return operations.NewRefreshTokenDefault(403).WithPayload(models.FromError(err))
	case err == nil:
		// all is ok
		return operations.NewRefreshTokenOK().WithPayload(token)
	default:
		// unknown error
		return operations.NewRefreshTokenDefault(500).WithPayload(models.FromError(err))
	}
}
