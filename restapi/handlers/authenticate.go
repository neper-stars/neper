package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewAuthenticateHandler returns an instance of Authenticate
func NewAuthenticateHandler(db *sqlx.DB, auth *auth.Auth) *Authenticate {
	return &Authenticate{db, auth}
}

// Authenticate handles /auth/authenticate
type Authenticate struct {
	db   *sqlx.DB
	auth *auth.Auth
}

func (h *Authenticate) handle(
	ctx context.Context, credentials *models.Credentials,
) (string, error) {
	log := *zerolog.Ctx(ctx)
	log.Debug().Str("credentials", fmt.Sprintf("%+v", credentials)).Msg("trying to auth with creds")
	principal, err := h.auth.Authenticate(ctx, credentials)
	if err != nil {
		log.Debug().Msg("failed to auth")
		return "", err
	}
	log.Debug().Str("user", principal.NickName).Msg("auth succeeded")
	return h.auth.MakeToken(*principal)
}

// Handle authenticates the user with the given credential and returns a JWT
func (h *Authenticate) Handle(
	params operations.AuthenticateParams,
) middleware.Responder {
	ctx := params.HTTPRequest.Context()
	log := *zerolog.Ctx(ctx)
	token, err := h.handle(ctx, params.Credentials)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			return operations.
				NewAuthenticateDefault(http.StatusUnauthorized).
				WithPayload(models.FromError(err))
		}
		// default error
		log.Err(err).Msg("authentication failed for unknown internal reason")
		return operations.
			NewAuthenticateDefault(http.StatusInternalServerError).
			WithPayload(models.FromError(err))
	}
	return operations.NewAuthenticateOK().WithPayload(token)
}
