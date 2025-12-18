package handlers

import (
	"context"

	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewInvitationSentListHandler creates a handler for listing invitations sent by the current user
func NewInvitationSentListHandler(log *zerolog.Logger, db *sqlx.DB) *InvitationSentList {
	return &InvitationSentList{db, log}
}

// InvitationSentList handles GET /invitations/sent
type InvitationSentList struct {
	db  *sqlx.DB
	log *zerolog.Logger
}

// invitationSentWithDetails is used for the JOIN query result
type invitationSentWithDetails struct {
	models.Invitation
	SessionName     string `db:"session_name"`
	InviteeNickname string `db:"invitee_nickname"`
}

func (h *InvitationSentList) handle(
	ctx context.Context, principal *models.Principal,
) ([]*models.Invitation, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	// Join with session and user_profile tables to get session_name and invitee nickname
	i := models.Schema.InvitationDB.As("i")
	s := models.Schema.SessionDB.As("s")
	u := models.Schema.UserProfileDB.As("u")

	q := database.SQ.Select().
		Column(i.ID.Sql()).
		Column(i.SessionID.Sql()).
		Column(i.UserProfileID.Sql()).
		Column(i.InviterID.Sql()).
		Column(s.Name.Sql() + " AS session_name").
		Column(u.Nickname.Sql() + " AS invitee_nickname").
		From(i.Sql()).
		LeftJoin(i.SessionID.Join(s.ID).Sql()).
		LeftJoin(i.UserProfileID.Join(u.ID).Sql()).
		Where(i.InviterID.Eq(principal.Subject)).
		OrderBy(i.ID.Sql())

	var list []invitationSentWithDetails
	if err := sqlH.Select(&list, q); err != nil {
		return nil, err
	}

	var retList []*models.Invitation
	for i := range list {
		inv := list[i].Invitation
		inv.SessionName = list[i].SessionName
		inv.InviteeNickname = list[i].InviteeNickname
		retList = append(retList, &inv)
	}
	return retList, nil
}

// Handle handles the request
func (h *InvitationSentList) Handle(
	params operations.InvitationSentListParams, principal *models.Principal,
) middleware.Responder {
	invitations, err := h.handle(params.HTTPRequest.Context(), principal)
	if err != nil {
		return operations.NewInvitationSentListDefault(500).WithPayload(models.FromError(err))
	}
	return operations.NewInvitationSentListOK().WithPayload(invitations)
}
