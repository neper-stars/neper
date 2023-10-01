package lib

import (
	"context"

	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/models"
)

// InitUserProfile is a helper to create all user relation entries in either our keto layer or
// the database layer
func InitUserProfile(ctx context.Context, h database.SQLHelper, userProfileDB *models.UserProfileDB, relations []*models.UserProfileSessionRelDB) error {
	log := zerolog.Ctx(ctx)
	// save relations in database for later use (like resynching a keto database)
	if err := syncUserProfileSessionRelations(
		h,
		models.UserProfileSessionRelDBTable,
		models.UserProfileSessionRelDBUserProfileIDColumn,
		models.UserProfileSessionRelDBSessionIDColumn,
		models.UserProfileSessionRelDBIsManagerColumn,
		userProfileDB.ID,
		relations,
	); err != nil {
		log.Err(err).
			Str("func", "InitUserProfile").
			Str("user", "@"+userProfileDB.Nickname).
			Msg("could not save user profile sessions")
		return err
	}
	return nil
}
