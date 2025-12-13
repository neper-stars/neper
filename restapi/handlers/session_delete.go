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

// NewSessionDeleteHandler creates a new session delete handler
func NewSessionDeleteHandler(log *zerolog.Logger, db *sqlx.DB, notifyService *notify.Service) *SessionDeleteHandler {
	return &SessionDeleteHandler{db: db, log: log, notifyService: notifyService}
}

// SessionDeleteHandler handles session deletion
type SessionDeleteHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	notifyService *notify.Service
}

func (h *SessionDeleteHandler) handle(
	ctx context.Context, params operations.SessionDeleteParams, principal *models.Principal,
) error {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Check if session exists
	var sessionDB models.SessionDB
	if err := sqlH.GetByPKey(&sessionDB, params.SessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewErrSessionNotFound("session not found: " + params.SessionID)
		}
		return err
	}

	// Authorization check
	authorized, err := h.Authorize(sqlH, params, principal)
	if err != nil {
		return err
	}
	if !authorized {
		return errs.ErrForbidden
	}

	// Delete the session (cascades to related tables via ON DELETE CASCADE)
	query := sq.Delete(models.SessionDBTable).
		Where(sq.Eq{models.SessionDBIDColumn: params.SessionID})

	result, err := sqlH.Exec(query)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errs.NewErrSessionNotFound("session not found: " + params.SessionID)
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Publish notification after successful commit
	if h.notifyService != nil {
		_ = h.notifyService.PublishSessionDelete(params.SessionID)
	}

	return nil
}

// Handle handles the request
func (h *SessionDeleteHandler) Handle(
	params operations.SessionDeleteParams, principal *models.Principal,
) middleware.Responder {
	err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionDeleteForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionDeleteNoContent()
}

// Authorize checks if the principal is allowed to delete this session
func (h *SessionDeleteHandler) Authorize(
	sqlH database.SQLHelper, params operations.SessionDeleteParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	return IsSessionManager(sqlH, principal.Subject, params.SessionID)
}
