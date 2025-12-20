package handlers

import (
	"context"
	"errors"
	"net/http"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewPendingRegistrationListHandler creates a new handler
func NewPendingRegistrationListHandler(log *zerolog.Logger, db *sqlx.DB) *PendingRegistrationListHandler {
	return &PendingRegistrationListHandler{db: db, log: log}
}

// PendingRegistrationListHandler handles GET /pending_registrations
type PendingRegistrationListHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *PendingRegistrationListHandler) handle(
	ctx context.Context, params operations.PendingRegistrationListParams, principal *models.Principal,
) ([]*models.UserProfile, error) {
	authorized, err := h.Authorize(ctx, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	log := *zerolog.Ctx(ctx)
	sqlH := database.NewSQLHelper(ctx, h.db, log)

	var usersDB []models.UserProfileDB
	query := database.SQ.
		Select(models.UserProfileDBColumns...).
		From(models.UserProfileDBTable).
		Where(sq.Eq{models.UserProfileDBPendingColumn: true}).
		OrderBy(models.UserProfileDBNicknameColumn + " ASC")

	if err := sqlH.Select(&usersDB, query); err != nil {
		return nil, err
	}

	result := make([]*models.UserProfile, len(usersDB))
	for i := range usersDB {
		result[i] = &usersDB[i].UserProfile
	}

	return result, nil
}

// Handle handles the request
func (h *PendingRegistrationListHandler) Handle(
	params operations.PendingRegistrationListParams, principal *models.Principal,
) middleware.Responder {
	users, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewPendingRegistrationListForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewPendingRegistrationListOK().WithPayload(users)
}

// Authorize checks if the principal is allowed to list pending registrations
func (h *PendingRegistrationListHandler) Authorize(
	ctx context.Context, params operations.PendingRegistrationListParams, principal *models.Principal,
) (bool, error) {
	// Only global managers can list pending registrations
	if principal.IsGlobalManager {
		return true, nil
	}
	return false, nil
}
