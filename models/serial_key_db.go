package models

import "time"

// SerialKeyDB stores a serial key
// dbtable:"serial_key" dbpkey:"key"
type SerialKeyDB struct {
	Key           string     `db:"key"`
	UserProfileID *string    `db:"user_profile_id"`
	AssignedAt    *time.Time `db:"assigned_at"`
}
