package handlers

import (
	"context"
	"errors"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/m4rw3r/uuid"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/lib/pq"
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
	ctx context.Context, invitation *models.Invitation, sessionID string, principal *models.Principal,
) (*models.Invitation, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	var invitationDB models.InvitationDB
	invitationUID, err := uuid.V4()
	if err != nil {
		return nil, err
	}
	// force our ID not the one from the client
	invitation.ID = invitationUID.String()
	// force session ID to be the one
	// or return an error if the ID is not the same ???
	invitation.SessionID = sessionID
	invitationDB.Invitation = *invitation

	_, err = sqlH.Insert(&invitationDB)
	if err != nil {
		pqErr, ok := err.(*pq.Error)
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
	params operations.SessionInviteParams, principal *models.Principal,
) middleware.Responder {
	invitation, err := h.handle(params.HTTPRequest.Context(), params.Invitation, params.SessionID, principal)
	if err != nil {
		switch {
		case errors.Is(err, errs.ErrNotFound):
			return NotFound(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		case errors.Is(err, errs.ErrInvalid):
			return BadRequest(err.Error(), zerolog.Ctx(params.HTTPRequest.Context()))
		default:
			return InternalError(err, zerolog.Ctx(params.HTTPRequest.Context()), false)
		}
	}
	return operations.NewSessionInviteCreated().WithPayload(invitation)
}
