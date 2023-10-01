package models

// UserProfileSessionRelDB is the relation table between a user profile and
// the sessions
// It also stores the user standing (manager or not) for the given relation
// dbtable:"user_profile_session_rel" dbpkey:"user_profile_id,session_id"
type UserProfileSessionRelDB struct {
	UserProfileID string `db:"user_profile_id"`
	SessionID     string `db:"session_id"`
	IsManager     bool   `db:"is_manager"`
}

func ToUserProfileSessionRelDB(sessionID string, members, managers []string) []*UserProfileSessionRelDB {
	var relations []*UserProfileSessionRelDB
	for _, member := range members {
		relations = append(relations, &UserProfileSessionRelDB{
			SessionID:     sessionID,
			IsManager:     false,
			UserProfileID: member,
		})
	}
	for _, manager := range managers {
		relations = append(relations, &UserProfileSessionRelDB{
			SessionID:     sessionID,
			IsManager:     true,
			UserProfileID: manager,
		})
	}
	return relations
}
