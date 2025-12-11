package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/m4rw3r/uuid"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewInvitationCreateHandler ...
func NewInvitationCreateHandler(log *zerolog.Logger, db *sqlx.DB) *InvitationCreateHandler {
	return &InvitationCreateHandler{db, log}
}

// InvitationCreateHandler handles /sessions
type InvitationCreateHandler struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

type ErrInvalidInvitation struct {
	Message string
}

func (e *ErrInvalidInvitation) Error() string {
	return e.Message
}

func (e *ErrInvalidInvitation) Is(target error) bool {
	return errors.Is(target, errs.ErrInvalid)
}

func (h *InvitationCreateHandler) handle(
	ctx context.Context, params operations.InvitationCreateParams, principal *models.Principal,
) (*models.Invitation, error) {
	authorized, err := h.Authorize(ctx, params, principal)
	if err != nil {
		return nil, err
	}
	if !authorized {
		return nil, errs.ErrForbidden
	}

	invitation := params.Invitation
	sessionID := params.SessionID

	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Fetch session to get session_name
	var sessionDB models.SessionDB
	if err := sqlH.GetByPKey(&sessionDB, sessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.ErrSessionNotFound{GivenMessage: "session not found"}
		}
		return nil, err
	}

	// Fetch inviter's user profile to get inviter_nickname
	var inviterDB models.UserProfileDB
	if err := sqlH.GetByPKey(&inviterDB, principal.Subject); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errs.ErrUserProfileNotFound{GivenMessage: "inviter user profile not found"}
		}
		return nil, err
	}

	var invitationDB models.InvitationDB
	invitationUID, err := uuid.V4()
	if err != nil {
		return nil, err
	}
	// force our ID not the one from the client
	invitation.ID = invitationUID.String()
	invitation.SessionID = sessionID
	invitation.InviterID = principal.Subject
	invitation.SessionName = sessionDB.Name
	invitation.InviterNickname = inviterDB.Nickname
	invitationDB.Invitation = *invitation

	_, err = sqlH.Insert(&invitationDB)
	if err != nil {
		// we don't want errorlint because we NEED to get the real pq.Error in order to extract the constraint
		pqErr, ok := err.(*pq.Error) // nolint:errorlint
		if ok {
			// we received an error from PG
			if pqErr.Constraint == "invitation_session_id_user_profile_id_key" {
				// and this error is specific to a constraint we know how to interpret as: Invitation already exists
				return nil, errs.NewErrAlreadyExists("invitation already exists for user: " + principal.Subject + "and session: " + sessionID)
			}
		}
		return invitation, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &invitationDB.Invitation, nil
}

// Handle handles the request
func (h *InvitationCreateHandler) Handle(
	params operations.InvitationCreateParams, principal *models.Principal,
) middleware.Responder {
	invitation, err := h.handle(params.HTTPRequest.Context(), params, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrForbidden):
			return operations.NewInvitationCreateForbidden().
				WithPayload(models.NewError(http.StatusForbidden, verbotten))
		case errors.Is(err, errs.ErrConflict):
			return operations.NewInvitationCreateConflict().
				WithPayload(models.NewError(http.StatusConflict, err.Error()))
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrInvalid):
			return BadRequest(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewInvitationCreateCreated().WithPayload(invitation)
}

func (h *InvitationCreateHandler) Authorize(
	ctx context.Context, params operations.InvitationCreateParams, principal *models.Principal,
) (bool, error) {
	if principal.IsGlobalManager {
		return true, nil
	}
	askedSessionID := params.SessionID

	// ensure the asked session ID is the same as the posted session ID in the body
	if params.SessionID != params.Invitation.SessionID {
		// this kind of trickery is Verboten
		h.log.Warn().Str("path", params.SessionID).Str("body", params.Invitation.SessionID).
			Msg("user tries to invite with different session ID in path and body")
		return false, nil
	}

	query := userProfileSessionRelationQuery(principal.Subject, askedSessionID)

	var authRes models.UserProfileSessionRelDB
	if err := database.GetContext(ctx, h.db, &authRes, query, h.log); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			h.log.Debug().Msg("auth: no matching userprofile session relation --> reject")
			return false, nil
		}
		return false, err
	}
	return authRes.IsManager, nil
}
