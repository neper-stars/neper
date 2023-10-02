package handlers

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewInvitationListHandler ...
func NewInvitationListHandler(log *zerolog.Logger, db *sqlx.DB) *InvitationList {
	return &InvitationList{db, log}
}

// InvitationList handles /sessions
type InvitationList struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

func (h *InvitationList) handle(
	ctx context.Context, principal *models.Principal,
) ([]*models.Invitation, error) {
	// no authorization here as everyone is allowed to list all invitations on which he is invited
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sql := database.NewSQLHelper(ctx, tx, log)

	q := sq.Select(models.InvitationColumns...).
		From(models.InvitationDBTable).
		Where(sq.Eq{models.InvitationDBUserProfileIDColumn: principal.Subject}).
		OrderBy(models.InvitationDBIDColumn)

	var list []*models.InvitationDB
	if err := sql.Select(&list, q); err != nil {
		return nil, err
	}

	var retList []*models.Invitation
	for i := range list {
		retList = append(retList, &list[i].Invitation)
	}
	return retList, nil
}

// Handle handles the request
func (h *InvitationList) Handle(
	params operations.InvitationListParams, principal *models.Principal,
) middleware.Responder {
	sessions, err := h.handle(params.HTTPRequest.Context(), principal)
	if err != nil {
		return operations.NewInvitationListDefault(500).WithPayload(models.FromError(err))
	}
	return operations.NewInvitationListOK().WithPayload(sessions)
}
