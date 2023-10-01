package handlers

import (
	"context"
	"errors"
	"slices"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/m4rw3r/uuid"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/lib"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewSessionCreateHandler ...
func NewSessionCreateHandler(db *sqlx.DB, authz *auth.Authorizer) *SessionCreateHandler {
	return &SessionCreateHandler{db, authz}
}

// SessionCreateHandler handles /sessions
type SessionCreateHandler struct {
	db    *sqlx.DB
	authz *auth.Authorizer
}

func (h *SessionCreateHandler) handle(
	ctx context.Context, session *models.Session, principal *models.Principal,
) (*models.Session, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	var sessionDB models.SessionDB
	sessionUID, err := uuid.V4()
	if err != nil {
		return nil, err
	}
	// force our ID not the one from the client
	session.ID = sessionUID.String()
	// scan managers to see if the creator already added his id as manager
	var alreadyManager bool
	for _, m := range session.Managers {
		if m == principal.Subject {
			alreadyManager = true
		}
	}
	for i, m := range session.Members {
		if m == principal.Subject {
			session.Members = slices.Delete(session.Members, i, i)
			// we found the subject in the members. remove it since it will
			// be in managers, this will avoid an error in the sql inserts
			break
		}
	}
	// if not add the creator ID as manager
	if !alreadyManager {
		// creator is added as a manager of the session
		session.Managers = append(session.Managers, principal.Subject)
	}

	sessionDB.Session = *session
	_, err = sqlH.Insert(&sessionDB)
	if err != nil {
		return session, err
	}

	// make sure the session sets its members/managers in database
	if err := lib.InitSession(
		ctx, sqlH,
		&sessionDB, models.ToUserProfileSessionRelDB(session.ID, session.Members, session.Managers),
	); err != nil {
		return session, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &sessionDB.Session, nil
}

// Handle handles the request
func (h *SessionCreateHandler) Handle(
	params operations.SessionCreateParams, principal *models.Principal,
) middleware.Responder {
	session, err := h.handle(params.HTTPRequest.Context(), params.Session, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionCreateCreated().WithPayload(session)
}
