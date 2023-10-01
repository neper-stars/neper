package models

import (
	"github.com/Masterminds/squirrel"
	"orus.io/orus-io/go-orusapi/database"
)

// UserProfileDB stores a user_profile
// dbtable:"user_profile" dbpkey:"id"
type UserProfileDB struct {
	UserProfile

	APIKey string `db:"apikey"`
}

// SessionIDList returns the list of Session ids for this userProfileDB
func (u *UserProfileDB) SessionIDList(sql *database.SQLHelper) ([]string, error) {
	var ids []string

	// get only the session ids
	query := squirrel.Select(UserProfileSessionRelDBSessionIDColumn).
		From(UserProfileSessionRelDBTable).
		Where(squirrel.Eq{UserProfileSessionRelDBUserProfileIDColumn: u.ID})

	if err := sql.Select(&ids, query); err != nil {
		return nil, err
	}
	return ids, nil
}
