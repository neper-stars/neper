package sync

import (
	"context"

	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/models/types"
)

func (w *Worker) syncSessionFiles(ctx context.Context, sql database.SQLHelper, op Operation, data *SessionFiles) error {
	if op == OpCreate || op == OpUpdate {
		var sessionFilesDB = models.SessionFilesDB{
			SessionFiles: models.SessionFiles{
				ID:        data.Id,
				SessionID: data.SessionId,
				Year:      int64(data.Year),
				HostFile:  data.Hostfile,
				Universe:  data.Universe,
				Turns:     dataTurnsToModelTurns(data.Turns),
				Orders:    dataOrdersToModelOrders(data.Orders),
			},
		}
		if err := sql.Upsert(&sessionFilesDB); err != nil {
			return err
		}
	}
	return nil
}

func dataTurnsToModelTurns(turns []*Turn) types.TurnList {
	var res types.TurnList
	for _, t := range turns {
		res = append(res, types.Turn{B64Data: t.B64Data})
	}
	return res
}

func dataOrdersToModelOrders(orders []*Order) types.OrderList {
	var res types.OrderList
	for _, t := range orders {
		res = append(res, types.Order{B64Data: t.B64Data})
	}
	return res
}
