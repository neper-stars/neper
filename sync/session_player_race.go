package sync

import (
	"context"

	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/models"
)

func (w *Worker) syncSessionPlayerRace(ctx context.Context, sql database.SQLHelper, op Operation, data *SessionPlayerRace) error {
	if op == OpCreate || op == OpUpdate {
		var sessionPlayerRaceDB = models.SessionPlayerRaceDB{
			SessionPlayerRace: models.SessionPlayerRace{
				ID:            data.Id,
				IsBot:         data.IsBot,
				PlayerOrder:   int64(data.PlayerOrder),
				RaceID:        data.RaceId,
				SessionID:     data.SessionId,
				UserProfileID: data.UserProfileId,
			},
		}
		if data.IsBot {
			botLevel := int64(data.BotLevel)
			sessionPlayerRaceDB.BotLevel = &botLevel
		}
		if err := sql.Upsert(&sessionPlayerRaceDB); err != nil {
			return err
		}
	}
	return nil
}
