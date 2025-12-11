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

// NewInvitationDeclineHandler ...
func NewInvitationDeclineHandler(log *zerolog.Logger, db *sqlx.DB) *InvitationDeclineHandler {
	return &InvitationDeclineHandler{db, log}
}

// InvitationDeclineHandler handles declining invitations
type InvitationDeclineHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *InvitationDeclineHandler) handle(
	ctx context.Context, params operations.InvitationDeclineParams, principal *models.Principal,
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

	// Delete the invitation
	query := sq.Delete(models.InvitationDBTable).
		Where(sq.Eq{models.InvitationDBIDColumn: params.InvitationID})

	result, err := sqlH.Exec(query)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errs.ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// Handle handles the request
func (h *InvitationDeclineHandler) Handle(
	params operations.InvitationDeclineParams, principal *models.Principal,
) middleware.Responder {
	err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewInvitationDeclineForbidden().
				WithPayload(models.NewError(http.StatusForbidden, verbotten))
		case errors.Is(err, errs.ErrNotFound):
			return operations.NewInvitationDeclineNotFound().
				WithPayload(models.NewError(http.StatusNotFound, "invitation not found"))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewInvitationDeclineNoContent()
}

func (h *InvitationDeclineHandler) Authorize(
	ctx context.Context, params operations.InvitationDeclineParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}

	query := invitationQuery(principal.Subject, params.InvitationID)

	var authRes models.InvitationDB
	if err := database.GetContext(ctx, h.db, &authRes, query, h.log); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.log.Debug().Msg("auth: no matching userprofile invitation --> reject")
			return false, nil
		}
		return false, err
	}
	return true, nil
}
