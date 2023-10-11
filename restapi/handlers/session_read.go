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

type SessionPrivateQueryResult struct {
	Private bool `db:"private"`
}

func (h *SessionReadHandler) Authorize(
	sqlH database.SQLHelper, params operations.SessionReadParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}

	askedSessionID := params.SessionID

	query := database.SQ.
		Select().
		Columns(
			models.SessionDBPrivateColumn,
		).
		From(models.SessionDBTable).
		Where(sq.Eq{models.SessionDBIDColumn: askedSessionID})

	var p SessionPrivateQueryResult
	if err := sqlH.Get(&p, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.log.Debug().Msg("auth: no matching session found --> reject")
			return false, nil
		}
		return false, err
	}

	// if session is public no problem
	if !p.Private {
		return true, nil
	}

	// if session is private only members have access
	return IsSessionMember(sqlH, principal.Subject, askedSessionID)
}
