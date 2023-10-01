package handlers

import (
	"context"
	"database/sql"
	"errors"

	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/models"
)

func VerifUserIsValid(ctx context.Context, db *sqlx.DB, log *zerolog.Logger, principal *models.Principal) (bool, error) {
	query := database.SQ.
		Select().
		Columns(
			models.UserProfileDBIDColumn,
			models.UserProfileDBNicknameColumn,
			models.UserProfileDBAPIKeyColumn,
			models.UserProfileDBIsActiveColumn,
		).
		From(models.UserProfileDBTable).
		Where(
			sq.Eq{models.UserProfileDBIDColumn: principal.Subject},
		)

	var userProfile models.UserProfileDB
	if err := database.GetContext(ctx, db, &userProfile, query, log); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Debug().Msg("auth: no matching profile")
			return false, auth.ErrInvalidCredentials
		}
		return false, err
	}
	if userProfile.APIKey != "" && userProfile.IsActive {
		return true, nil
	}
	return false, nil
}
