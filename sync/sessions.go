package sync

import (
	"context"

	"orus.io/orus-io/go-orusapi/database"

	neper "github.com/neper-stars/neper/lib"
	"github.com/neper-stars/neper/models"
)

func (w *Worker) syncSession(ctx context.Context, sql database.SQLHelper, op Operation, data *Session) error {
	if op == OpCreate || op == OpUpdate {
		var sessionDB = models.SessionDB{
			Session: models.Session{
				ID:         data.Id,
				Name:       data.Name,
				Private:    data.Private,
				Started:    data.Started,
				RulesIsSet: data.RulesIsSet,
			},
		}
		if err := sql.Upsert(&sessionDB); err != nil {
			return err
		}

		var relations []*models.UserProfileSessionRelDB
		for _, member := range data.Members {
			relations = append(relations, &models.UserProfileSessionRelDB{
				SessionID:     sessionDB.ID,
				IsManager:     member.IsManager,
				UserProfileID: member.UserProfileId,
			})
		}

		if err := neper.InitSession(ctx, sql, &sessionDB, relations); err != nil {
			return err
		}
	}
	return nil
}
