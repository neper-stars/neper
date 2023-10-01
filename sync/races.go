package sync

import (
	"context"

	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/models"
)

func (w *Worker) syncRace(ctx context.Context, sql database.SQLHelper, op Operation, data *Race) error {
	if op == OpCreate || op == OpUpdate {
		var raceDB = models.RaceDB{
			Race: models.Race{
				ID:           data.Id,
				UserID:       data.UserId,
				NamePlural:   data.NamePlural,
				NameSingular: data.NameSingular,
				Data:         data.Data,
			},
		}
		if err := sql.Upsert(&raceDB); err != nil {
			return err
		}
	}
	return nil
}
