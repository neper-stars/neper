package models

// RulesetDB ...
// dbtable:"ruleset" dbpkey:"id"
type RulesetDB struct {
	Ruleset
	ID        string `db:"id"`
	SessionID string `db:"session_id"`
}
