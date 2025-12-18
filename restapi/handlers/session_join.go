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

// NewSessionJoinHandler creates a new handler for joining a public session
func NewSessionJoinHandler(log *zerolog.Logger, db *sqlx.DB, notifyService *notify.Service) *SessionJoinHandler {
	return &SessionJoinHandler{db: db, log: log, notifyService: notifyService}
}

// SessionJoinHandler handles POST /sessions/{session_id}/join
type SessionJoinHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	notifyService *notify.Service
}

func (h *SessionJoinHandler) handle(
	ctx context.Context, params operations.SessionJoinParams, principal *models.Principal,
) (*models.Session, error) {
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

	// Get the session
	var sessionDB models.SessionDB
	sessionID := params.SessionID
	if err := sqlH.GetByPKey(&sessionDB, sessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.NewErrSessionNotFound("session not found with ID: " + sessionID)
		}
		return nil, err
	}

	// Insert the membership
	relation := models.UserProfileSessionRelDB{
		SessionID:     sessionID,
		IsManager:     false,
		UserProfileID: principal.Subject,
	}
	_, err = sqlH.Insert(&relation)
	if err != nil {
		pqErr, ok := err.(*pq.Error) // nolint:errorlint
		if ok {
			// Handle both possible constraint names for duplicate membership
			if pqErr.Constraint == "user_profile_session_rel_user_profile_id_session_id_key" ||
				pqErr.Constraint == "user_profile_session_rel_pkey" {
				return nil, errs.NewErrAlreadyExists("session membership already exists for user: " + principal.Subject + " and session: " + sessionID)
			}
		}
		log.Err(err).Msg("failed to insert relation upon session join")
		return nil, err
	}

	// Load all relations after adding ours
	if err := (&sessionDB).FromDB(&sqlH); err != nil {
		return nil, err
	}

	// Delete any existing invitation for this user and session
	var deletedInvitationID string
	invitationQuery := database.SQ.Select(models.InvitationDBIDColumn).
		From(models.InvitationDBTable).
		Where(sq.And{
			sq.Eq{models.InvitationDBUserProfileIDColumn: principal.Subject},
			sq.Eq{models.InvitationDBSessionIDColumn: sessionID},
		})
	if err := sqlH.Get(&deletedInvitationID, invitationQuery); err == nil {
		// Found an invitation, delete it
		deleteQuery := database.SQ.Delete(models.InvitationDBTable).
			Where(sq.Eq{models.InvitationDBIDColumn: deletedInvitationID})
		if _, err := sqlH.Exec(deleteQuery); err != nil {
			log.Err(err).Msg("failed to delete invitation upon session join")
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Publish notifications after successful commit
	if h.notifyService != nil {
		_ = h.notifyService.PublishSessionUpdate(sessionID)
		if deletedInvitationID != "" {
			_ = h.notifyService.PublishInvitationDelete(deletedInvitationID, principal.Subject)
		}
	}

	return &sessionDB.Session, nil
}

// Handle handles the request
func (h *SessionJoinHandler) Handle(
	params operations.SessionJoinParams, principal *models.Principal,
) middleware.Responder {
	session, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionJoinForbidden().
				WithPayload(models.NewError(http.StatusForbidden, verbotten))
		case errors.Is(err, errs.ErrConflict):
			return operations.NewSessionJoinDefault(http.StatusConflict).
				WithPayload(models.NewError(http.StatusConflict, err.Error()))
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrInvalid):
			return BadRequest(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionJoinOK().WithPayload(session)
}

// Authorize checks if the user can join this session (public or has invitation)
func (h *SessionJoinHandler) Authorize(
	sqlH database.SQLHelper, params operations.SessionJoinParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}

	// Check if session is public
	isPublic, err := IsSessionPublic(sqlH, params.SessionID)
	if err != nil {
		return false, err
	}
	if isPublic {
		return true, nil
	}

	// Private session: check if user has a pending invitation
	isInvited, err := IsInvitedToSession(sqlH, principal.Subject, params.SessionID)
	if err != nil {
		return false, err
	}
	if isInvited {
		return true, nil
	}

	h.log.Debug().
		Str("session_id", params.SessionID).
		Str("user_id", principal.Subject).
		Msg("auth: cannot join private session without invitation --> reject")
	return false, nil
}
