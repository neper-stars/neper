package handlers

import (
	"context"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	neper "github.com/neper-stars/neper/lib"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

// NewUserProfilesListHandler ...
func NewUserProfilesListHandler(db *sqlx.DB) *UserProfilesList {
	return &UserProfilesList{db}
}

// UserProfilesList handles /sessions
type UserProfilesList struct {
	db *sqlx.DB
}

func (h *UserProfilesList) handle(
	ctx context.Context, principal *models.Principal,
) ([]*models.UserProfile, error) {
	log := *zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(log)
	sql := database.NewSQLHelper(ctx, tx, log)

	u := models.Schema.UserProfileDB.As("u")

	query := database.SQ.Select().
		Column(u.ID.Sql()).
		Column(u.Nickname.Sql()).
		Column(u.Email.Sql()).
		Column(u.IsActive.Sql()).
		Column(u.IsManager.Sql()).
		From(u.Sql()).
		Where(sq.NotEq{models.UserProfileDBIDColumn: neper.SystemUserID}).
		OrderBy(u.ID.Sql())

	var list []*models.UserProfileDB
	if err := sql.Select(&list, query); err != nil {
		return nil, err
	}

	var retList []*models.UserProfile
	for i := range list {
		retList = append(retList, &list[i].UserProfile)
	}
	return retList, nil
}

// Handle handles the request
func (h *UserProfilesList) Handle(
	params operations.UserProfileListParams, principal *models.Principal,
) middleware.Responder {
	users, err := h.handle(params.HTTPRequest.Context(), principal)
	if err != nil {
		return operations.NewSessionsListDefault(500).WithPayload(models.FromError(err))
	}
	return operations.NewUserProfileListOK().WithPayload(users)
}
