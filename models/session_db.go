package models

import (
	"github.com/Masterminds/squirrel"
	"orus.io/orus-io/go-orusapi/database"
)

// SessionDB stores a session
// dbtable:"session" dbpkey:"id"
type SessionDB struct {
	Session
}

// MembersID returns the list of Member ids
func (c *SessionDB) MembersID(sql *database.SQLHelper) ([]string, error) {
	var ids []string

	if err := sql.Select(&ids,
		squirrel.Select(UserProfileSessionRelDBUserProfileIDColumn).
			From(UserProfileSessionRelDBTable).
			Where(squirrel.And{
				squirrel.Eq{UserProfileSessionRelDBSessionIDColumn: c.ID},
				squirrel.Eq{UserProfileSessionRelDBIsManagerColumn: false},
			}),
	); err != nil {
		return nil, err
	}
	return ids, nil
}

// ManagersID returns the list of Manager ids
func (c *SessionDB) ManagersID(sql *database.SQLHelper) ([]string, error) {
	var ids []string

	if err := sql.Select(&ids,
		squirrel.Select(UserProfileSessionRelDBUserProfileIDColumn).
			From(UserProfileSessionRelDBTable).
			Where(squirrel.And{
				squirrel.Eq{UserProfileSessionRelDBSessionIDColumn: c.ID},
				squirrel.Eq{UserProfileSessionRelDBIsManagerColumn: true},
			}),
	); err != nil {
		return nil, err
	}
	return ids, nil
}

// PlayersID returns the list of Players ids sorted by player order
func (c *SessionDB) PlayersID(sql *database.SQLHelper) ([]string, error) {
	var ids []string

	if err := sql.Select(&ids,
		squirrel.Select(SessionPlayerRaceDBUserProfileIDColumn).
			From(SessionPlayerRaceDBTable).
			Where(squirrel.And{
				squirrel.Eq{SessionPlayerRaceDBSessionIDColumn: c.ID},
			}).OrderBy(SessionPlayerRaceDBPlayerOrderColumn+" ASC"),
	); err != nil {
		return nil, err
	}
	return ids, nil
}

// FromDB loads the data stored in other tables than 'user_profile' in the API
// facing attributes
func (c *SessionDB) FromDB(db *database.SQLHelper) error {
	ids, err := c.MembersID(db)
	if err != nil {
		return err
	}
	c.Members = ids

	ids, err = c.ManagersID(db)
	if err != nil {
		return err
	}
	c.Managers = ids

	ids, err = c.PlayersID(db)
	if err != nil {
		return err
	}
	c.Players = ids

	return nil
}
