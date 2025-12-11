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

// invitationWithDetails is used for the JOIN query result
type invitationWithDetails struct {
	models.Invitation
	SessionName     string `db:"session_name"`
	InviterNickname string `db:"inviter_nickname"`
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
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Join with session and user_profile tables to get session_name and inviter_nickname
	q := sq.Select(
		"i."+models.InvitationDBIDColumn,
		"i."+models.InvitationDBSessionIDColumn,
		"i."+models.InvitationDBUserProfileIDColumn,
		"i."+models.InvitationDBInviterIDColumn,
		"s.name AS session_name",
		"u.nickname AS inviter_nickname",
	).
		From(models.InvitationDBTable+" AS i").
		LeftJoin(models.SessionDBTable+" AS s ON i."+models.InvitationDBSessionIDColumn+" = s.id").
		LeftJoin(models.UserProfileDBTable+" AS u ON i."+models.InvitationDBInviterIDColumn+" = u.id").
		Where(sq.Eq{"i." + models.InvitationDBUserProfileIDColumn: principal.Subject}).
		OrderBy("i." + models.InvitationDBIDColumn)

	var list []invitationWithDetails
	if err := sqlH.Select(&list, q); err != nil {
		return nil, err
	}

	var retList []*models.Invitation
	for i := range list {
		inv := list[i].Invitation
		inv.SessionName = list[i].SessionName
		inv.InviterNickname = list[i].InviterNickname
		retList = append(retList, &inv)
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
