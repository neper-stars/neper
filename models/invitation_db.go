package models

// InvitationDB stores an invitation for a user to a session
// dbtable:"invitation" dbpkey:"id"
type InvitationDB struct {
	Invitation
}
