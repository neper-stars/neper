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

// NewSessionsListHandler ...
func NewSessionsListHandler(db *sqlx.DB) *SessionsList {
	return &SessionsList{db}
}

// SessionsList handles /sessions
type SessionsList struct {
	db *sqlx.DB
}

func (h *SessionsList) handle(
	ctx context.Context, principal *models.Principal,
) ([]*models.Session, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sqlH := database.NewSQLHelper(ctx, tx, log)

	s := models.Schema.SessionDB.As("s")

	// Select all SessionDB columns to ensure full session objects are returned
	query := database.SQ.Select().
		Columns(s.FQColumns(true)...).
		From(s.Sql()).
		OrderBy(s.ID.Sql())

	// Track which sessions have pending invitations
	var invitedSessionIDs map[string]bool

	if !principal.IsGlobalManager {
		// filter using the sessions membership if the user is not a global manager
		upsr := models.Schema.UserProfileSessionRelDB.As("upsr")
		inv := models.Schema.InvitationDB.As("inv")

		// Use LEFT JOINs so public sessions without members/invitations are still returned
		query = query.LeftJoin(s.ID.Join(upsr.SessionID).Sql())
		// Join invitation table with extra condition for user_profile_id
		query = query.LeftJoin(
			s.ID.Join(inv.SessionID).Sql()+" AND "+inv.UserProfileID.Sql()+" = ?",
			principal.Subject,
		)

		filter := sq.Or{
			// public sessions: visible to everyone
			sq.Eq{models.SessionDBPrivateColumn: false},
			// private sessions: only if I am a member
			sq.And{
				sq.Eq{models.SessionDBPrivateColumn: true},
				upsr.UserProfileID.Eq(principal.Subject),
			},
			// private sessions: also visible if I have a pending invitation
			sq.And{
				sq.Eq{models.SessionDBPrivateColumn: true},
				sq.NotEq{inv.ID.Sql(): nil},
			},
		}
		query = query.Where(filter).Distinct()

		// Query invited session IDs separately to know which ones to flag
		invitedSessionIDs, _ = h.getInvitedSessionIDs(sqlH, principal.Subject)
	}

	var list []*models.SessionDB
	if err := sqlH.Select(&list, query); err != nil {
		return nil, err
	}

	var retList []*models.Session
	for i := range list {
		// load all related tables to populate session
		if err := list[i].FromDB(&sqlH); err != nil {
			return nil, err
		}

		// Set pending_invitation flag if user has invitation and is not a member
		if invitedSessionIDs != nil && invitedSessionIDs[list[i].ID] {
			// Check if user is a member (managers and members are in the session)
			isMember := false
			for _, memberID := range list[i].Members {
				if memberID == principal.Subject {
					isMember = true
					break
				}
			}
			if !isMember {
				for _, managerID := range list[i].Managers {
					if managerID == principal.Subject {
						isMember = true
						break
					}
				}
			}
			if !isMember {
				list[i].PendingInvitation = true
			}
		}

		retList = append(retList, &list[i].Session)
	}
	return retList, nil
}

// getInvitedSessionIDs returns a map of session IDs that the user has pending invitations for
func (h *SessionsList) getInvitedSessionIDs(sqlH database.SQLHelper, userProfileID string) (map[string]bool, error) {
	query := database.SQ.Select(models.InvitationDBSessionIDColumn).
		From(models.InvitationDBTable).
		Where(sq.Eq{models.InvitationDBUserProfileIDColumn: userProfileID})

	var sessionIDs []string
	if err := sqlH.Select(&sessionIDs, query); err != nil {
		return nil, err
	}

	result := make(map[string]bool)
	for _, id := range sessionIDs {
		result[id] = true
	}
	return result, nil
}

// Handle handles the request
func (h *SessionsList) Handle(
	params operations.SessionsListParams, principal *models.Principal,
) middleware.Responder {
	sessions, err := h.handle(params.HTTPRequest.Context(), principal)
	if err != nil {
		return operations.NewSessionsListDefault(500).WithPayload(models.FromError(err))
	}
	return operations.NewSessionsListOK().WithPayload(sessions)
}
