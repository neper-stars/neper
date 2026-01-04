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

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewSessionArchiveHandler creates a new session archive handler
func NewSessionArchiveHandler(log *zerolog.Logger, db *sqlx.DB, notifyService *notify.Service) *SessionArchiveHandler {
	return &SessionArchiveHandler{db: db, log: log, notifyService: notifyService}
}

// SessionArchiveHandler handles POST /sessions/{session_id}/archive
type SessionArchiveHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	notifyService *notify.Service
}

func (h *SessionArchiveHandler) handle(
	ctx context.Context, params operations.SessionArchiveParams, principal *models.Principal,
) (*models.Session, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Check if session exists
	var sessionDB models.SessionDB
	if err := sqlH.GetByPKey(&sessionDB, params.SessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSessionNotFound("session not found: " + params.SessionID)
		}
		return nil, err
	}

	// Authorization check - only managers or global managers can archive
	authorized, err := h.Authorize(sqlH, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	// Can only archive started sessions
	if sessionDB.State != models.SessionStateStarted {
		return nil, errs.NewErrPreconditionFailed("can only archive sessions that are started")
	}

	// Update the session state to archived
	sessionDB.State = models.SessionStateArchived
	if err := sqlH.Update(&sessionDB); err != nil {
		return nil, err
	}

	// Load all relations after updating
	if err := (&sessionDB).FromDB(&sqlH); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Publish notification after successful commit
	if h.notifyService != nil {
		_ = h.notifyService.PublishSessionUpdate(params.SessionID)
	}

	return &sessionDB.Session, nil
}

// Handle handles the request
func (h *SessionArchiveHandler) Handle(
	params operations.SessionArchiveParams, principal *models.Principal,
) middleware.Responder {
	session, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionArchiveForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrPreconditionFailed):
			return PreconditionFailed(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionArchiveOK().WithPayload(session)
}

// Authorize checks if the principal is allowed to archive this session
func (h *SessionArchiveHandler) Authorize(
	sqlH database.SQLHelper, params operations.SessionArchiveParams, principal *models.Principal,
) (bool, error) {
	// Pending users cannot archive sessions
	if principal.IsPending {
		return false, nil
	}
	if principal.IsGlobalManager {
		return true, nil
	}
	return IsSessionManager(sqlH, principal.Subject, params.SessionID)
}
