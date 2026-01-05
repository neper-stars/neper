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

// NewSessionPlayerRaceDeleteHandler creates a new handler
func NewSessionPlayerRaceDeleteHandler(db *sqlx.DB, notifyService *notify.Service) *SessionPlayerRaceDeleteHandler {
	return &SessionPlayerRaceDeleteHandler{db: db, notifyService: notifyService}
}

// SessionPlayerRaceDeleteHandler handles player race deletion from sessions
type SessionPlayerRaceDeleteHandler struct {
	db            *sqlx.DB
	notifyService *notify.Service
}

func (h *SessionPlayerRaceDeleteHandler) handle(
	ctx context.Context, params operations.SessionPlayerRaceDeleteParams, principal *models.Principal,
) error {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Fetch the session player race entry
	var spr models.SessionPlayerRaceDB
	if err := sqlH.GetByPKey(&spr, params.PlayerRaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewErrSomethingNotFound("player race not found with id: " + params.PlayerRaceID)
		}
		return err
	}

	// Verify it belongs to the specified session
	if spr.SessionID != params.SessionID {
		return errs.NewErrSomethingNotFound("player race not found in this session")
	}

	// Check authorization
	isManager, err := IsSessionManager(sqlH, principal.Subject, params.SessionID)
	if err != nil {
		return err
	}

	canDelete := false
	if principal.IsGlobalManager || isManager {
		// Managers can delete anyone
		canDelete = true
	} else if !spr.IsBot && spr.UserProfileID == principal.Subject {
		// Regular users can only delete themselves (and only if not a bot)
		canDelete = true
	}

	if !canDelete {
		return errs.ErrForbidden
	}

	// Check ready status for human players
	if !spr.IsBot && spr.Ready {
		return errs.NewErrInvalidSomething("cannot remove a ready player, they must unready first")
	}

	// Delete the entry
	_, err = sqlH.Exec(database.SQ.
		Delete(models.SessionPlayerRaceDBTable).
		Where(sq.Eq{models.SessionPlayerRaceDBIDColumn: params.PlayerRaceID}))
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Publish notification
	if h.notifyService != nil {
		_ = h.notifyService.PublishSessionPlayerRaceDelete(params.PlayerRaceID)
	}

	return nil
}

// Handle handles the request
func (h *SessionPlayerRaceDeleteHandler) Handle(
	params operations.SessionPlayerRaceDeleteParams, principal *models.Principal,
) middleware.Responder {
	err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionPlayerRaceDeleteForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrInvalid):
			return BadRequest(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionPlayerRaceDeleteNoContent()
}
