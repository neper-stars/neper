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

	query := database.SQ.Select().
		Column(s.ID.Sql()).
		Column(s.Name.Sql()).
		Column(s.Private.Sql()).
		From(s.Sql()).
		OrderBy(s.ID.Sql())

	if !principal.IsGlobalManager {
		// filter using the sessions membership if the user is not a global manager
		upsr := models.Schema.UserProfileSessionRelDB.As("upsr")
		// Use LEFT JOIN so public sessions without any members are still returned
		query = query.LeftJoin(s.ID.Join(upsr.SessionID).Sql())

		filter := sq.Or{
			// public sessions: visible to everyone
			sq.Eq{models.SessionDBPrivateColumn: false},
			// private sessions: only if I am a member
			sq.And{
				sq.Eq{models.SessionDBPrivateColumn: true},
				upsr.UserProfileID.Eq(principal.Subject),
			},
		}
		query = query.Where(filter).Distinct()
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
		retList = append(retList, &list[i].Session)
	}
	return retList, nil
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
