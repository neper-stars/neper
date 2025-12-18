package sync

import (
	"context"

	"orus.io/orus-io/go-orusapi/database"

	neper "github.com/neper-stars/neper/lib"
	"github.com/neper-stars/neper/models"
)

func (w *Worker) syncUserProfile(ctx context.Context, sql database.SQLHelper, op Operation, data *UserProfile) error {
	if op == OpCreate || op == OpUpdate {
		// sanity check
		if data.IsActive && data.ApiKey == "" {
			w.logger.Warn().
				Str("userID", data.Id).
				Str("email", data.Email).
				Msg("user is marked as active but has no api_key set... will not work as expected")
		}

		var serialKey *string
		if data.SerialKey != "" {
			serialKey = &data.SerialKey
		}

		var userProfileDB = models.UserProfileDB{
			UserProfile: models.UserProfile{
				ID:        data.Id,
				Nickname:  data.Nickname,
				Email:     data.Email,
				IsActive:  data.IsActive,
				IsManager: data.IsManager,
			},
			APIKey:    data.ApiKey,
			SerialKey: serialKey,
		}
		if err := sql.Upsert(&userProfileDB); err != nil {
			w.logger.Err(err).
				Str("func", "syncUserProfile").
				Str("nickname", data.Nickname).
				Msg("could not save user profile")
			return err
		}

		var relations []*models.UserProfileSessionRelDB
		for _, session := range data.SessionList {
			relations = append(relations, &models.UserProfileSessionRelDB{
				SessionID:     session.SessionId,
				IsManager:     session.IsManager,
				UserProfileID: userProfileDB.ID,
			})
		}

		if err := neper.InitUserProfile(ctx, sql, &userProfileDB, relations); err != nil {
			return err
		}
	}
	return nil
}
