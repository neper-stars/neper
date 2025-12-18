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

// NewSessionReadHandler ...
func NewSessionReadHandler(log *zerolog.Logger, db *sqlx.DB) *SessionReadHandler {
	return &SessionReadHandler{db, log}
}

// SessionReadHandler handles /session
type SessionReadHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *SessionReadHandler) handle(
	ctx context.Context, params operations.SessionReadParams, principal *models.Principal,
) (*models.Session, error) {
	sessionID := params.SessionID

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// ** Authorization **
	authorized, err := h.Authorize(sqlH, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

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

	// Check if user has pending invitation (not yet a member)
	if !principal.IsGlobalManager {
		isMember, err := IsSessionMember(sqlH, principal.Subject, sessionID)
		if err != nil {
			return nil, err
		}
		if !isMember {
			isInvited, err := IsInvitedToSession(sqlH, principal.Subject, sessionID)
			if err != nil {
				return nil, err
			}
			sessionDB.Session.PendingInvitation = isInvited
		}
	}

	return &sessionDB.Session, nil
}

// Handle handles the request
func (h *SessionReadHandler) Handle(
	params operations.SessionReadParams, principal *models.Principal,
) middleware.Responder {
	session, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionReadForbidden().WithPayload(&models.Error{
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
	return operations.NewSessionReadOK().WithPayload(session)
}

func (h *SessionReadHandler) Authorize(
	sqlH database.SQLHelper, params operations.SessionReadParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	return CanAccessSession(sqlH, principal.Subject, params.SessionID)
}
