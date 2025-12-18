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

// userWithSerial is a helper struct to query user data with serial_key
type userWithSerial struct {
	models.User
	SerialKey *string `db:"serial_key"`
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
		Select(models.UserIDColumn, models.UserNicknameColumn, "serial_key").
		From(models.UserProfileDBTable).
		Where(sq.Eq{models.UserProfileDBIDColumn: principal.Subject})

	var user userWithSerial
	if err := database.Get(tx, &user, userQuery, log); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUnknownUser
		}
		return nil, err
	}

	result := &models.Userinfo{
		User: &user.User,
	}
	if user.SerialKey != nil {
		result.SerialKey = *user.SerialKey
	}

	return result, nil
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
