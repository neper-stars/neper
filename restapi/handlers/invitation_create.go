package handlers

import (
	"context"
	"errors"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/m4rw3r/uuid"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewInvitationCreateHandler ...
func NewInvitationCreateHandler(db *sqlx.DB) *InvitationCreateHandler {
	return &InvitationCreateHandler{db}
}

// InvitationCreateHandler handles /sessions
type InvitationCreateHandler struct {
	db *sqlx.DB
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
