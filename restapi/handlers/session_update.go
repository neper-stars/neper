package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	neper "github.com/neper-stars/neper/lib"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewSessionUpdateHandler ...
func NewSessionUpdateHandler(log *zerolog.Logger, db *sqlx.DB) *SessionUpdateHandler {
	return &SessionUpdateHandler{db, log}
}

// SessionUpdateHandler handles /circles
type SessionUpdateHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *SessionUpdateHandler) handle(
	ctx context.Context, params operations.SessionUpdateParams, principal *models.Principal,
) (*models.Session, error) {
	session := params.Session

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	authorized, err := h.Authorize(sqlH, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	var sessionDB models.SessionDB

	sessionDB.Session = *session
	err = sqlH.Upsert(&sessionDB)
	if err != nil {
		return session, err
	}

	// make sure the session sets its members/managers in database
	if err := neper.InitSession(
		ctx, sqlH, &sessionDB,
		models.ToUserProfileSessionRelDB(session.ID, session.Members, session.Managers),
	); err != nil {
		return session, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &sessionDB.Session, nil
}

// Handle handles the request
func (h *SessionUpdateHandler) Handle(
	params operations.SessionUpdateParams, principal *models.Principal,
) middleware.Responder {
	session, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionUpdateForbidden().WithPayload(&models.Error{
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
	return operations.NewSessionUpdateOK().WithPayload(session)
}

func (h *SessionUpdateHandler) Authorize(
	sqlH database.SQLHelper, params operations.SessionUpdateParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	return IsSessionManager(sqlH, principal.Subject, params.SessionID)
}
