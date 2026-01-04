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
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewInvitationDeclineHandler ...
func NewInvitationDeclineHandler(log *zerolog.Logger, db *sqlx.DB, notifyService *notify.Service) *InvitationDeclineHandler {
	return &InvitationDeclineHandler{db: db, log: log, notifyService: notifyService}
}

// InvitationDeclineHandler handles declining invitations
type InvitationDeclineHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	notifyService *notify.Service
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

	// Get the invitation first to capture the user profile ID for notification
	var invitationDB models.InvitationDB
	if err := sqlH.GetByPKey(&invitationDB, params.InvitationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.ErrNotFound
		}
		return err
	}

	// Delete the invitation
	query := database.SQ.Delete(models.InvitationDBTable).
		Where(sq.Eq{models.InvitationDBIDColumn: params.InvitationID})

	if _, err := sqlH.Exec(query); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Publish notification after successful commit
	if h.notifyService != nil {
		_ = h.notifyService.PublishInvitationDelete(params.InvitationID, invitationDB.UserProfileID)
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
	// Pending users cannot decline invitations
	if principal.IsPending {
		return false, nil
	}
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
