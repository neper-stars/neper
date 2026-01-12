package models

// SessionPlayerRaceDB stores the race settings for a user profile
// on a specific session
// dbtable:"session_player_race" dbpkey:"id"
type SessionPlayerRaceDB struct {
	SessionPlayerRace

	// AIControlType tracks mid-game control changes.
	// NULL = human controlled
	// HE/SS/IS/CA/PP/AR = AI controlled with that expert type
	// This is separate from IsBot which tracks initial player type (bot vs human slot).
	AIControlType *string `json:"-" db:"ai_control_type"`
}
