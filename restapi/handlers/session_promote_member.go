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
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewSessionPromoteMemberHandler creates a new handler for promoting a member to manager
func NewSessionPromoteMemberHandler(log *zerolog.Logger, db *sqlx.DB, notifyService *notify.Service) *SessionPromoteMemberHandler {
	return &SessionPromoteMemberHandler{db: db, log: log, notifyService: notifyService}
}

// SessionPromoteMemberHandler handles POST /sessions/{session_id}/promote/{user_profile_id}
type SessionPromoteMemberHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	notifyService *notify.Service
}

func (h *SessionPromoteMemberHandler) handle(
	ctx context.Context, params operations.SessionPromoteMemberParams, principal *models.Principal,
) error {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	sessionID := params.SessionID
	targetUserID := params.UserProfileID

	// Check if session exists
	var sessionDB models.SessionDB
	if err := sqlH.GetByPKey(&sessionDB, sessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewErrSessionNotFound("session not found with ID: " + sessionID)
		}
		return err
	}

	// Authorization: must be session manager or global manager
	authorized, err := h.Authorize(sqlH, params, principal)
	if err != nil {
		return err
	}
	if !authorized {
		return errs.ErrForbidden
	}

	// Check if target user is a member of the session
	var relation models.UserProfileSessionRelDB
	if err := sqlH.GetWhere(&relation, sq.And{
		sq.Eq{models.UserProfileSessionRelDBUserProfileIDColumn: targetUserID},
		sq.Eq{models.UserProfileSessionRelDBSessionIDColumn: sessionID},
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewErrNotAMember("user is not a member of this session")
		}
		return err
	}

	// If already a manager, nothing to do
	if relation.IsManager {
		// Already a manager, just return success
		return nil
	}

	// Update the membership to manager
	updateQuery := database.SQ.Update(models.UserProfileSessionRelDBTable).
		Set(models.UserProfileSessionRelDBIsManagerColumn, true).
		Where(sq.And{
			sq.Eq{models.UserProfileSessionRelDBUserProfileIDColumn: targetUserID},
			sq.Eq{models.UserProfileSessionRelDBSessionIDColumn: sessionID},
		})

	if _, err := sqlH.Exec(updateQuery); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Publish notification after successful commit
	if h.notifyService != nil {
		_ = h.notifyService.PublishSessionUpdate(sessionID)
	}

	return nil
}

// Authorize checks if the principal is allowed to promote members in this session
func (h *SessionPromoteMemberHandler) Authorize(
	sqlH database.SQLHelper, params operations.SessionPromoteMemberParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	return IsSessionManager(sqlH, principal.Subject, params.SessionID)
}

// Handle handles the request
func (h *SessionPromoteMemberHandler) Handle(
	params operations.SessionPromoteMemberParams, principal *models.Principal,
) middleware.Responder {
	err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionPromoteMemberForbidden().
				WithPayload(models.NewError(http.StatusForbidden, verbotten))
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrPreconditionFailed):
			return operations.NewSessionPromoteMemberPreconditionFailed().
				WithPayload(models.NewError(http.StatusPreconditionFailed, err.Error()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionPromoteMemberNoContent()
}
