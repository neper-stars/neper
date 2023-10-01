package handlers

import (
	"context"
	"database/sql"
	"errors"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewSessionReadHandler ...
func NewSessionReadHandler(db *sqlx.DB) *SessionReadHandler {
	return &SessionReadHandler{db}
}

// SessionReadHandler handles /session
type SessionReadHandler struct {
	db *sqlx.DB
}

func (h *SessionReadHandler) handle(
	ctx context.Context, sessionID string,
) (*models.Session, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	var sessionDB models.SessionDB
	if err := sqlH.GetByPKey(&sessionDB, sessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSessionNotFound("session not found with ID: " + sessionID)
		}
		return nil, err
	}
	// load all details from other tables
	if err := (&sessionDB).FromDB(&sqlH); err != nil {
		return nil, err
	}

	return &sessionDB.Session, nil
}

// Handle handles the request
func (h *SessionReadHandler) Handle(
	params operations.SessionReadParams, principal *models.Principal,
) middleware.Responder {
	session, err := h.handle(params.HTTPRequest.Context(), params.SessionID)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionReadOK().WithPayload(session)
}
