package models

import (
	sq "github.com/Masterminds/squirrel"
	"orus.io/orus-io/go-orusapi/database"
)

// SessionDB stores a session
// dbtable:"session" dbpkey:"id"
type SessionDB struct {
	Session
	// RulesetID is the FK to the ruleset table, only stored in DB (not exposed in API)
	RulesetID *string `db:"ruleset_id"`
}

// MembersID returns the list of Member ids
func (c *SessionDB) MembersID(sql *database.SQLHelper) ([]string, error) {
	var ids []string

	if err := sql.Select(&ids,
		sq.Select(UserProfileSessionRelDBUserProfileIDColumn).
			From(UserProfileSessionRelDBTable).
			Where(sq.And{
				sq.Eq{UserProfileSessionRelDBSessionIDColumn: c.ID},
				sq.Eq{UserProfileSessionRelDBIsManagerColumn: false},
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
		sq.Select(UserProfileSessionRelDBUserProfileIDColumn).
			From(UserProfileSessionRelDBTable).
			Where(sq.And{
				sq.Eq{UserProfileSessionRelDBSessionIDColumn: c.ID},
				sq.Eq{UserProfileSessionRelDBIsManagerColumn: true},
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
		sq.Select(SessionPlayerRaceDBUserProfileIDColumn).
			From(SessionPlayerRaceDBTable).
			Where(sq.And{
				sq.Eq{SessionPlayerRaceDBSessionIDColumn: c.ID},
			}).OrderBy(SessionPlayerRaceDBPlayerOrderColumn+" ASC"),
	); err != nil {
		return nil, err
	}
	return ids, nil
}

// sessionPlayerInfo is a helper struct for querying player info
type sessionPlayerInfo struct {
	UserProfileID string `db:"user_profile_id"`
	Ready         bool   `db:"ready"`
	PlayerOrder   int64  `db:"player_order"`
	IsBot         bool   `db:"is_bot"`
	BotLevel      *int64 `db:"bot_level"`
}

// PlayersInfo returns the list of Players with their ready status, sorted by player order
func (c *SessionDB) PlayersInfo(sql *database.SQLHelper) ([]*SessionPlayer, error) {
	var infos []sessionPlayerInfo

	if err := sql.Select(&infos,
		sq.Select(
			SessionPlayerRaceDBUserProfileIDColumn,
			SessionPlayerRaceDBReadyColumn,
			SessionPlayerRaceDBPlayerOrderColumn,
			SessionPlayerRaceDBIsBotColumn,
			SessionPlayerRaceDBBotLevelColumn,
		).
			From(SessionPlayerRaceDBTable).
			Where(sq.And{
				sq.Eq{SessionPlayerRaceDBSessionIDColumn: c.ID},
			}).OrderBy(SessionPlayerRaceDBPlayerOrderColumn+" ASC"),
	); err != nil {
		return nil, err
	}

	var players []*SessionPlayer
	for _, info := range infos {
		players = append(players, &SessionPlayer{
			UserProfileID: info.UserProfileID,
			Ready:         info.Ready,
			PlayerOrder:   info.PlayerOrder,
			IsBot:         info.IsBot,
			BotLevel:      info.BotLevel,
		})
	}
	return players, nil
}

func (c *SessionDB) SessionPlayerRaces(sql *database.SQLHelper) ([]SessionPlayerRace, error) {
	var sessionPlayerRacesDB []SessionPlayerRaceDB
	if err := sql.Select(&sessionPlayerRacesDB,
		sq.Select(SessionPlayerRaceDBColumns...).
			From(SessionPlayerRaceDBTable).
			Where(sq.And{
				sq.Eq{SessionPlayerRaceDBSessionIDColumn: c.ID},
			}).OrderBy(SessionPlayerRaceDBPlayerOrderColumn+" ASC"),
	); err != nil {
		return nil, err
	}
	var sessionPlayerRaces []SessionPlayerRace
	for i := range sessionPlayerRacesDB {
		sessionPlayerRaces = append(sessionPlayerRaces, sessionPlayerRacesDB[i].SessionPlayerRace)
	}
	return sessionPlayerRaces, nil
}

// PlayerRaces takes a slice of SessionPlayerRace in a certain order and fetches all the corresponding
// races. Our usage is generally to have this list ordered by playerOrder. But your mileage may vary.
func (c *SessionDB) PlayerRaces(sql *database.SQLHelper, sprs []SessionPlayerRace) ([]Race, error) {
	var raceIDs []string
	for _, spr := range sprs {
		if spr.IsBot {
			continue
		}
		raceIDs = append(raceIDs, spr.RaceID)
	}
	var racesDB []RaceDB
	if err := sql.Select(&racesDB,
		sq.Select(RaceDBColumns...).
			From(RaceDBTable).
			Where(sq.Eq{RaceDBIDColumn: raceIDs}),
	); err != nil {
		return nil, err
	}

	var races []Race
	// reorg list by player order at the same time we produce the result list
	for _, spr := range sprs {
		if spr.IsBot {
			continue
		}
		// get the race that corresponds the player
		for i := range racesDB {
			if racesDB[i].UserID == spr.UserProfileID {
				races = append(races, racesDB[i].Race)
			}
		}
	}
	return races, nil
}

func (c *SessionDB) Ruleset(sql *database.SQLHelper) (*Ruleset, error) {
	var rulesetDB RulesetDB

	if err := sql.GetBy(&rulesetDB, RulesetDBSessionIDColumn, c.ID); err != nil {
		return nil, err
	}

	return &rulesetDB.Ruleset, nil
}

// FromDB loads the data stored in other tables than 'session' in the API
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

	players, err := c.PlayersInfo(db)
	if err != nil {
		return err
	}
	c.Players = players

	return nil
}
