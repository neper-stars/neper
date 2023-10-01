package handlers

import (
	"context"
	"database/sql"
	"errors"

	sq "github.com/Masterminds/squirrel"
	"github.com/go-openapi/runtime/middleware"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

var ErrUnknownUser = errors.New("unknown user")

// NewUserinfoHandler returns a Userinfo handler
func NewUserinfoHandler(db *sqlx.DB) *Userinfo {
	return &Userinfo{db}
}

// Userinfo handles /newspapers/{id}
type Userinfo struct {
	db *sqlx.DB
}

func (h *Userinfo) handle(
	ctx context.Context, principal *models.Principal,
) (*models.Userinfo, error) {
	log := zerolog.Ctx(ctx)
	tx, err := database.Begin(ctx, h.db)
	if err != nil {
		return nil, err
	}
	defer tx.RollbackIfOpened(*log)

	userQuery := database.SQ.
		Select(models.UserColumns...).
		From(models.UserProfileDBTable).
		Where(sq.Eq{models.UserProfileDBIDColumn: principal.Subject})

	var user models.User
	if err := database.Get(tx, &user, userQuery, log); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUnknownUser
		}
		return nil, err
	}

	return &models.Userinfo{
		User: &user,
	}, nil
}

// Handle an incoming request
func (h *Userinfo) Handle(
	params operations.UserinfoParams, principal *models.Principal,
) middleware.Responder {
	if info, err := h.handle(params.HTTPRequest.Context(), principal); err != nil {
		return operations.NewUserinfoDefault(500).WithPayload(models.FromError(err))
	} else if info != nil {
		return operations.NewUserinfoOK().WithPayload(info)
	}
	return operations.NewUserinfoDefault(500)
}
