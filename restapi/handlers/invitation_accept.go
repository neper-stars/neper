package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewInvitationAcceptHandler ...
func NewInvitationAcceptHandler(log *zerolog.Logger, db *sqlx.DB, notifyService *notify.Service) *InvitationAcceptHandler {
	return &InvitationAcceptHandler{db: db, log: log, notifyService: notifyService}
}

// InvitationAcceptHandler handles /sessions
type InvitationAcceptHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	notifyService *notify.Service
}

func (h *InvitationAcceptHandler) handle(
	ctx context.Context, params operations.InvitationAcceptParams, principal *models.Principal,
) (*models.Session, error) {
	authorized, err := h.Authorize(ctx, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	var invitationDB models.InvitationDB
	query := invitationQuery(principal.Subject, params.InvitationID)
	if err := sqlH.Get(&invitationDB, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.ErrForbidden
		}
		return nil, err
	}
	var sessionDB models.SessionDB
	sessionID := invitationDB.SessionID
	if err := sqlH.GetByPKey(&sessionDB, sessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.ErrSessionNotFound{GivenMessage: "session not found"}
		}
		// the sql server is misbehaving / we have a code problem this is a 500 error
		return nil, err
	}

	// Check if session has already started
	if sessionDB.Started {
		return nil, errs.NewErrSessionAlreadyStarted("cannot join a session that has already started")
	}

	// insert our new membership
	relation := models.UserProfileSessionRelDB{
		SessionID:     sessionID,
		IsManager:     false,
		UserProfileID: principal.Subject,
	}
	_, err = sqlH.Insert(&relation)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) {
			// we received an error from PG
			if pqErr.Constraint == "user_profile_session_rel_user_profile_id_session_id_key" {
				// and this error is specific to a constraint we know how to interpret as: session membership already exists
				return nil, errs.NewErrAlreadyExists("session membership already exists for user: " + principal.Subject + "and session: " + sessionID)
			}
		}
		log.Err(err).Msg("failed to insert relation upon invitation accept")
		return nil, err
	}

	// load all relations after adding ours
	if err := (&sessionDB).FromDB(&sqlH); err != nil {
		return nil, err
	}

	// Once everything else is done we remove invite from db
	deleteQuery := database.SQ.Delete(models.InvitationDBTable).
		Where(sq.Eq{models.InvitationDBIDColumn: params.InvitationID})

	result, err := sqlH.Exec(deleteQuery)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, errs.ErrNotFound
	}

	// and finally commit the whole transaction as a whole
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Publish notifications after successful commit
	if h.notifyService != nil {
		_ = h.notifyService.PublishInvitationDelete(params.InvitationID, invitationDB.UserProfileID)
		_ = h.notifyService.PublishSessionUpdate(sessionID)
	}

	return &sessionDB.Session, nil
}

// Handle handles the request
func (h *InvitationAcceptHandler) Handle(
	params operations.InvitationAcceptParams, principal *models.Principal,
) middleware.Responder {
	session, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewInvitationAcceptForbidden().
				WithPayload(models.NewError(http.StatusForbidden, verbotten))
		case errors.Is(err, errs.ErrConflict):
			return operations.NewInvitationAcceptConflict().
				WithPayload(models.NewError(http.StatusConflict, err.Error()))
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrInvalid):
			return BadRequest(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrPreconditionFailed):
			return PreconditionFailed(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewInvitationAcceptOK().WithPayload(session)
}

func (h *InvitationAcceptHandler) Authorize(
	ctx context.Context, params operations.InvitationAcceptParams, principal *models.Principal,
) (bool, error) {
	// Should we really allow a general manager to accept
	// invites on behalf of someone else ?
	if principal.IsGlobalManager {
		return true, nil
	}

	query := invitationQuery(principal.Subject, params.InvitationID)

	var authRes models.InvitationDB
	if err := database.GetContext(ctx, h.db, &authRes, query, h.log); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.log.Debug().Msg("auth: no matching userprofile invitation --> reject")
			return false, nil
		}
		return false, err
	}
	return true, nil
}
