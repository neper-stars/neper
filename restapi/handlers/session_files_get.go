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
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewSessionFilesGetHandler creates a new handler for getting all session files
func NewSessionFilesGetHandler(log *zerolog.Logger, db *sqlx.DB) *SessionFilesGetHandler {
	return &SessionFilesGetHandler{db: db, log: log}
}

// SessionFilesGetHandler handles GET /sessions/{session_id}/files
// This endpoint is restricted to session managers and global managers only.
type SessionFilesGetHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *SessionFilesGetHandler) handle(
	ctx context.Context, params operations.SessionFilesGetParams, principal *models.Principal,
) (*models.SessionFiles, error) {
	sessionID := params.SessionID

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// ** Authorization - managers only **
	authorized, err := h.Authorize(sqlH, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	// Query for the latest session files (highest year)
	query := lastSessionFilesQuery(sessionID)
	var sessionFiles models.SessionFilesDB
	if err := sqlH.Get(&sessionFiles, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSomethingNotFound("no session files found for session: " + sessionID)
		}
		return nil, err
	}

	return &sessionFiles.SessionFiles, nil
}

// Handle handles the request
func (h *SessionFilesGetHandler) Handle(
	params operations.SessionFilesGetParams, principal *models.Principal,
) middleware.Responder {
	sessionFiles, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionFilesGetForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return operations.NewSessionFilesGetNotFound().WithPayload(models.FromError(err))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionFilesGetOK().WithPayload(sessionFiles)
}

// Authorize checks if the user is a session manager or global manager
func (h *SessionFilesGetHandler) Authorize(
	sqlH database.SQLHelper, params operations.SessionFilesGetParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	// Only session managers can access all session files
	return IsSessionManager(sqlH, principal.Subject, params.SessionID)
}
