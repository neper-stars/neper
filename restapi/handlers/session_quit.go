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

// NewSessionQuitHandler creates a new handler for leaving a session
func NewSessionQuitHandler(log *zerolog.Logger, db *sqlx.DB, notifyService *notify.Service) *SessionQuitHandler {
	return &SessionQuitHandler{db: db, log: log, notifyService: notifyService}
}

// SessionQuitHandler handles POST /sessions/{session_id}/quit
type SessionQuitHandler struct {
	db            *sqlx.DB
	log           *zerolog.Logger
	notifyService *notify.Service
}

func (h *SessionQuitHandler) handle(
	ctx context.Context, params operations.SessionQuitParams, principal *models.Principal,
) error {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	sessionID := params.SessionID

	// Check if session exists
	var sessionDB models.SessionDB
	if err := sqlH.GetByPKey(&sessionDB, sessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewErrSessionNotFound("session not found with ID: " + sessionID)
		}
		return err
	}

	// Check if session has already started
	if sessionDB.Started {
		return errs.NewErrSessionAlreadyStarted("cannot leave session: session has already started")
	}

	// Check if user is a member of the session
	var relation models.UserProfileSessionRelDB
	if err := sqlH.GetWhere(&relation, sq.And{
		sq.Eq{models.UserProfileSessionRelDBUserProfileIDColumn: principal.Subject},
		sq.Eq{models.UserProfileSessionRelDBSessionIDColumn: sessionID},
	}); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errs.NewErrSomethingNotFound("user is not a member of this session")
		}
		return err
	}

	// If the user is a manager, check that there's at least one other manager
	if relation.IsManager {
		otherManagerCount, err := h.countOtherManagers(sqlH, sessionID, principal.Subject)
		if err != nil {
			return err
		}
		if otherManagerCount == 0 {
			return errs.NewErrLastManager("cannot leave session: you are the only manager")
		}
	}

	// Check if player is ready - ready players cannot leave
	isReady, err := IsPlayerReady(sqlH, sessionID, principal.Subject)
	if err != nil {
		return err
	}
	if isReady {
		return errs.NewErrPlayerShouldNotBeReady("cannot leave session: you are marked as ready")
	}

	// Delete the session player race association if it exists
	deleteSPRQuery := database.SQ.Delete(models.SessionPlayerRaceDBTable).
		Where(sq.And{
			sq.Eq{models.SessionPlayerRaceDBUserProfileIDColumn: principal.Subject},
			sq.Eq{models.SessionPlayerRaceDBSessionIDColumn: sessionID},
		})

	if _, err := sqlH.Exec(deleteSPRQuery); err != nil {
		return err
	}

	// Delete the membership
	deleteQuery := database.SQ.Delete(models.UserProfileSessionRelDBTable).
		Where(sq.And{
			sq.Eq{models.UserProfileSessionRelDBUserProfileIDColumn: principal.Subject},
			sq.Eq{models.UserProfileSessionRelDBSessionIDColumn: sessionID},
		})

	if _, err := sqlH.Exec(deleteQuery); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Publish notification after successful commit
	if h.notifyService != nil {
		_ = h.notifyService.PublishSessionMemberLeft(sessionID, principal.Subject, sessionDB.Private)
	}

	return nil
}

// countOtherManagers returns the number of managers in the session excluding the given user
func (h *SessionQuitHandler) countOtherManagers(sqlH database.SQLHelper, sessionID, excludeUserID string) (int, error) {
	query := database.SQ.Select("COUNT(*)").
		From(models.UserProfileSessionRelDBTable).
		Where(sq.And{
			sq.Eq{models.UserProfileSessionRelDBSessionIDColumn: sessionID},
			sq.Eq{models.UserProfileSessionRelDBIsManagerColumn: true},
			sq.NotEq{models.UserProfileSessionRelDBUserProfileIDColumn: excludeUserID},
		})

	var count int
	if err := sqlH.Get(&count, query); err != nil {
		return 0, err
	}
	return count, nil
}

// Handle handles the request
func (h *SessionQuitHandler) Handle(
	params operations.SessionQuitParams, principal *models.Principal,
) middleware.Responder {
	err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewSessionQuitForbidden().
				WithPayload(models.NewError(http.StatusForbidden, verbotten))
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrPreconditionFailed):
			return operations.NewSessionQuitPreconditionFailed().
				WithPayload(models.NewError(http.StatusPreconditionFailed, err.Error()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionQuitNoContent()
}
