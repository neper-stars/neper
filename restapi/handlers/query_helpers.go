package handlers

import (
	"errors"

	sq "github.com/Masterminds/squirrel"
	"orus.io/orus-io/go-orusapi/database"

	"database/sql"

	"github.com/neper-stars/neper/models"
)

// IsSessionMember returns true if the given userProfileID is member of the given sessionID
// sql.ErrNoRows errors are caught and returned as false
// all other sql errors will return as normal errors (ie: can be treated as 500)
func IsSessionMember(sqlH database.SQLHelper, userProfileID, sessionID string) (bool, error) {
	var sessionRel models.UserProfileSessionRelDB
	if err := sqlH.GetWhere(&sessionRel, sessionMembersFilter(userProfileID, sessionID)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// session has no relation for the given user... --> refuse without error
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// IsSessionManager returns true if the given userProfileID is manager of the given sessionID
// sql.ErrNoRows errors are caught and returned as false
// all other sql errors will return as normal errors (ie: can be treated as 500)
func IsSessionManager(sqlH database.SQLHelper, userProfileID, sessionID string) (bool, error) {
	var sessionRel models.UserProfileSessionRelDB
	if err := sqlH.GetWhere(&sessionRel, sessionMembersFilter(userProfileID, sessionID)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// session has no relation for the given user... --> refuse without error
			return false, nil
		}
		return false, err
	}
	return sessionRel.IsManager, nil
}

func sessionMembersFilter(userProfileID, sessionID string) sq.And {
	return sq.And{
		sq.Eq{models.UserProfileSessionRelDBSessionIDColumn: sessionID},
		sq.Eq{models.UserProfileSessionRelDBUserProfileIDColumn: userProfileID},
	}
}

func userProfileSessionRelationQuery(userProfileID, sessionID string) sq.SelectBuilder {
	return database.SQ.
		Select().
		Columns(
			models.UserProfileSessionRelDBUserProfileIDColumn,
			models.UserProfileSessionRelDBSessionIDColumn,
			models.UserProfileSessionRelDBIsManagerColumn,
		).
		From(models.UserProfileSessionRelDBTable).
		Where(
			sq.And{
				sq.Eq{models.UserProfileSessionRelDBUserProfileIDColumn: userProfileID},
				sq.Eq{models.UserProfileSessionRelDBSessionIDColumn: sessionID},
			},
		)
}

func invitationQuery(userProfileID, invitationID string) sq.SelectBuilder {
	return database.SQ.
		Select().
		Columns(models.InvitationDBColumns...).
		From(models.InvitationDBTable).
		Where(
			sq.And{
				sq.Eq{models.InvitationDBIDColumn: invitationID},
				sq.Eq{models.InvitationDBUserProfileIDColumn: userProfileID},
			},
		)
}
