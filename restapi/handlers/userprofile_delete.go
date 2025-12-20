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
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewUserProfileDeleteHandler creates a new handler for deleting user profiles
func NewUserProfileDeleteHandler(log *zerolog.Logger, db *sqlx.DB) *UserProfileDeleteHandler {
	return &UserProfileDeleteHandler{db: db, log: log}
}

// UserProfileDeleteHandler handles DELETE /user_profiles/{user_profile_id}
type UserProfileDeleteHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *UserProfileDeleteHandler) handle(
	ctx context.Context, params operations.UserProfileDeleteParams, principal *models.Principal,
) error {
	authorized, err := h.Authorize(ctx, params, principal)
	if err != nil {
		return err
	}
	if !authorized {
		return errs.ErrForbidden
	}

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Check if the user profile exists
	var userDB models.UserProfileDB
	if err := sqlH.GetByPKey(&userDB, params.UserProfileID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewErrSomethingNotFound("user profile not found")
		}
		return err
	}

	// Check if the user is a member of any session
	isMember, err := IsMemberOfAnySession(sqlH, params.UserProfileID)
	if err != nil {
		return err
	}
	if isMember {
		return errs.NewErrAlreadyExists("user is a member of one or more sessions")
	}

	// Delete the user profile
	// Note: invitations and races are automatically deleted via ON DELETE CASCADE
	_, err = sqlH.Exec(database.SQ.
		Delete(models.UserProfileDBTable).
		Where(sq.Eq{models.UserProfileDBIDColumn: params.UserProfileID}))
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Info().
		Str("user_id", params.UserProfileID).
		Str("nickname", userDB.Nickname).
		Str("deleted_by", principal.Subject).
		Msg("user profile deleted")

	return nil
}

// Handle handles the request
func (h *UserProfileDeleteHandler) Handle(
	params operations.UserProfileDeleteParams, principal *models.Principal,
) middleware.Responder {
	err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewUserProfileDeleteForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrConflict):
			msg := err.Error()
			return operations.NewUserProfileDeleteConflict().WithPayload(&models.Error{
				Code:    http.StatusConflict,
				Message: &msg,
			})
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewUserProfileDeleteNoContent()
}

// Authorize checks if the principal is allowed to delete user profiles
func (h *UserProfileDeleteHandler) Authorize(
	ctx context.Context, params operations.UserProfileDeleteParams, principal *models.Principal,
) (bool, error) {
	// Only global managers can delete user profiles
	if principal.IsGlobalManager {
		return true, nil
	}
	return false, nil
}
