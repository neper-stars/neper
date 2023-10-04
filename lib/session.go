package neper

import (
	"context"

	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/models"
)

// InitSession is a helper to set all relations into the database layer
func InitSession(ctx context.Context, h database.SQLHelper, sessionDB *models.SessionDB, relations []*models.UserProfileSessionRelDB) error {
	log := zerolog.Ctx(ctx)

	// sync members into the database
	if err := syncUserProfileSessionRelations(
		h,
		models.UserProfileSessionRelDBTable,
		models.UserProfileSessionRelDBSessionIDColumn,
		models.UserProfileSessionRelDBUserProfileIDColumn,
		models.UserProfileSessionRelDBIsManagerColumn,
		sessionDB.ID,
		relations,
	); err != nil {
		log.Err(err).
			Str("func", "InitSession").
			Str("session", sessionDB.ID+" "+sessionDB.Name).
			Msg("could not save session members/managers")
		return err
	}
	return nil
}
