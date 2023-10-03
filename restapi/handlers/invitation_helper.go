package handlers

import (
	sq "github.com/Masterminds/squirrel"
	"orus.io/orus-io/go-orusapi/database"

	"github.com/neper-stars/neper/models"
)

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
