package handlers

import (
	"context"
	"errors"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/lib"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewSessionUpdateHandler ...
func NewSessionUpdateHandler(db *sqlx.DB, authz *auth.Authorizer) *SessionUpdateHandler {
	return &SessionUpdateHandler{db, authz}
}

// SessionUpdateHandler handles /circles
type SessionUpdateHandler struct {
	db    *sqlx.DB
	authz *auth.Authorizer
}

func (h *SessionUpdateHandler) handle(
	ctx context.Context, session *models.Session,
) (*models.Session, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	var sessionDB models.SessionDB

	sessionDB.Session = *session
	err = sqlH.Upsert(&sessionDB)
	if err != nil {
		return session, err
	}

	// make sure the session sets its members/managers in keto AND in database
	if err := lib.InitSession(
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
	circle, err := h.handle(params.HTTPRequest.Context(), params.Session)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionUpdateOK().WithPayload(circle)
}
