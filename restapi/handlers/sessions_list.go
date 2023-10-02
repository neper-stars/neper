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
	sql := database.NewSQLHelper(ctx, tx, log)

	s := models.Schema.SessionDB.As("s")

	query := database.SQ.Select().
		Column(s.ID.Sql()).
		Column(s.Name.Sql()).
		From(s.Sql()).
		OrderBy(s.ID.Sql())

	if !principal.IsGlobalManager {
		// filter using the sessions membership if the user is not a global manager
		upsr := models.Schema.UserProfileSessionRelDB.As("upsr")
		query = query.InnerJoin(s.ID.Join(upsr.SessionID).Sql())

		filter := sq.Or{
			sq.And{
				sq.Eq{models.SessionDBPrivateColumn: false},
				sq.Or{
					upsr.UserProfileID.Eq(principal.Subject),
					upsr.UserProfileID.Eq(nil),
				},
			}, // session is public, and (I am already in it or not, but not someone else)
			sq.Or{
				sq.Eq{models.SessionDBPrivateColumn: true},
				upsr.UserProfileID.Eq(principal.Subject),
			}, // session is private and I am a member
		}
		query = query.Where(filter)
	}

	var list []*models.SessionDB
	if err := sql.Select(&list, query); err != nil {
		return nil, err
	}

	var retList []*models.Session
	for i := range list {
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
