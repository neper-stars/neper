package neper

import (
	"github.com/Masterminds/squirrel"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/models"
)

// syncUserProfileSessionRelations ...
func syncUserProfileSessionRelations(
	h database.SQLHelper,
	table string, colFrom string, colIDTo string, colManagerTo string,
	colFromValue string, colToValues []*models.UserProfileSessionRelDB,
) error {
	if len(colToValues) != 0 {
		// Upsert all the given values
		q := squirrel.Insert(table).
			Columns(colFrom, colIDTo, colManagerTo)
		for _, value := range colToValues {
			if colIDTo == models.UserProfileSessionRelDBSessionIDColumn {
				q = q.Values(colFromValue, value.SessionID, value.IsManager)
			} else {
				q = q.Values(colFromValue, value.UserProfileID, value.IsManager)
			}
		}
		// on conflict update is_manager only
		q = q.Suffix(
			"ON CONFLICT (" +
				"user_profile_id, session_id" +
				") DO UPDATE SET " +
				models.UserProfileSessionRelDBIsManagerColumn +
				" = EXCLUDED.is_manager",
		)
		if _, err := h.Exec(q); err != nil {
			return err
		}
	}

	filter := squirrel.And{squirrel.Eq{colFrom: colFromValue}}
	for _, value := range colToValues {
		if colIDTo == models.UserProfileSessionRelDBSessionIDColumn {
			filter = append(filter, squirrel.NotEq{colIDTo: value.SessionID})
		} else {
			filter = append(filter, squirrel.NotEq{colIDTo: value.UserProfileID})
		}
	}
	// Delete unwanted values
	if _, err := h.Exec(squirrel.Delete(table).Where(filter)); err != nil {
		return err
	}
	return nil
}
