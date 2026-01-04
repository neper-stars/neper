package handlers

import (
	"context"
	"errors"
	"net/http"
	"slices"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/m4rw3r/uuid"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	neper "github.com/neper-stars/neper/lib"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/lib/session"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewSessionCreateHandler ...
func NewSessionCreateHandler(db *sqlx.DB, notifyService *notify.Service, opts *session.Options) *SessionCreateHandler {
	return &SessionCreateHandler{db: db, notifyService: notifyService, opts: opts}
}

// SessionCreateHandler handles /sessions
type SessionCreateHandler struct {
	db            *sqlx.DB
	notifyService *notify.Service
	opts          *session.Options
}

func (h *SessionCreateHandler) handle(
	ctx context.Context, params operations.SessionCreateParams, principal *models.Principal,
) (*models.Session, error) {
	authorized, err := h.Authorize(ctx, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	session := params.Session

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Check session membership limit (not for global managers)
	// Archived sessions don't count towards the limit
	if h.opts != nil && !principal.IsGlobalManager {
		var count int
		err := sqlH.Get(&count, database.SQ.
			Select("COUNT(*)").
			From(models.UserProfileSessionRelDBTable).
			Join(models.SessionDBTable+" ON "+
				models.SessionDBTable+"."+models.SessionDBIDColumn+" = "+
				models.UserProfileSessionRelDBTable+"."+models.UserProfileSessionRelDBSessionIDColumn).
			Where(sq.And{
				sq.Eq{models.UserProfileSessionRelDBUserProfileIDColumn: principal.Subject},
				sq.NotEq{models.SessionDBTable + "." + models.SessionDBStateColumn: models.SessionStateArchived},
			}))
		if err != nil {
			return nil, err
		}

		if count >= h.opts.MembershipLimit {
			return nil, errs.NewErrSessionLimitExceeded("session membership limit exceeded")
		}
	}

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
	if err := neper.InitSession(
		ctx, sqlH,
		&sessionDB, models.ToUserProfileSessionRelDB(session.ID, session.Members, session.Managers),
	); err != nil {
		return session, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Publish notification after successful commit
	if h.notifyService != nil {
		_ = h.notifyService.PublishSessionCreate(session.ID)
	}

	return &sessionDB.Session, nil
}

// Handle handles the request
func (h *SessionCreateHandler) Handle(
	params operations.SessionCreateParams, principal *models.Principal,
) middleware.Responder {
	session, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionCreateForbidden().WithPayload(&models.Error{
				Code:    http.StatusForbidden,
				Message: &verbotten,
			})
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrInvalid):
			return BadRequest(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrSessionLimitExceeded):
			msg := err.Error()
			return operations.NewSessionCreateTooManyRequests().WithPayload(&models.Error{
				Code:    http.StatusTooManyRequests,
				Message: &msg,
			})
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionCreateCreated().WithPayload(session)
}
func (h *SessionCreateHandler) Authorize(
	ctx context.Context, params operations.SessionCreateParams, principal *models.Principal,
) (bool, error) {
	// Pending users cannot create sessions
	if principal.IsPending {
		return false, nil
	}
	// Approved users are allowed to create sessions
	return true, nil
}
