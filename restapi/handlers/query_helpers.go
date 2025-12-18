package handlers

import (
	"database/sql"
	"errors"

	sq "github.com/Masterminds/squirrel"
	"github.com/rs/zerolog"
	"orus.io/orus-io/go-orusapi/database"

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

// IsSessionPublic returns true if the given session is public (not private)
// Returns false if session is private or not found
func IsSessionPublic(sqlH database.SQLHelper, sessionID string) (bool, error) {
	query := database.SQ.
		Select(models.SessionDBPrivateColumn).
		From(models.SessionDBTable).
		Where(sq.Eq{models.SessionDBIDColumn: sessionID})

	var result struct {
		Private bool `db:"private"`
	}
	if err := sqlH.Get(&result, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return !result.Private, nil
}

// IsInvitedToSession returns true if the given user has a pending invitation to the session
func IsInvitedToSession(sqlH database.SQLHelper, userProfileID, sessionID string) (bool, error) {
	query := database.SQ.
		Select(models.InvitationDBIDColumn).
		From(models.InvitationDBTable).
		Where(sq.And{
			sq.Eq{models.InvitationDBUserProfileIDColumn: userProfileID},
			sq.Eq{models.InvitationDBSessionIDColumn: sessionID},
		})

	var invitationID string
	if err := sqlH.Get(&invitationID, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CanAccessSession checks if a user can access a session.
// Access is granted if:
// - Session is public
// - User is a member of the session
// - User has a pending invitation to the session
func CanAccessSession(sqlH database.SQLHelper, userProfileID, sessionID string) (bool, error) {
	// Check if session is public
	isPublic, err := IsSessionPublic(sqlH, sessionID)
	if err != nil {
		return false, err
	}
	if isPublic {
		return true, nil
	}

	// Check if user is a member
	isMember, err := IsSessionMember(sqlH, userProfileID, sessionID)
	if err != nil {
		return false, err
	}
	if isMember {
		return true, nil
	}

	// Check if user has a pending invitation
	return IsInvitedToSession(sqlH, userProfileID, sessionID)
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

func sessionPlayerRaceQuery(userProfileID, sessionID string) sq.SelectBuilder {
	return database.SQ.
		Select().
		Columns(models.SessionPlayerRaceDBColumns...).
		From(models.SessionPlayerRaceDBTable).
		Where(
			sq.And{
				sq.Eq{models.SessionPlayerRaceDBUserProfileIDColumn: userProfileID},
				sq.Eq{models.SessionPlayerRaceDBSessionIDColumn: sessionID},
			},
		)
}

func sessionPlayerRaceForSessionQuery(sessionID string) sq.SelectBuilder {
	return database.SQ.
		Select().
		Columns(models.SessionPlayerRaceDBColumns...).
		From(models.SessionPlayerRaceDBTable).
		Where(
			sq.Eq{models.SessionPlayerRaceDBSessionIDColumn: sessionID},
		)
}

func lastSessionFilesQuery(sessionID string) sq.SelectBuilder {
	return database.SQ.
		Select().
		Columns(models.SessionFilesDBColumns...).
		From(models.SessionFilesDBTable).
		Where(
			sq.And{
				sq.Eq{models.SessionFilesDBSessionIDColumn: sessionID},
			},
		).OrderBy(models.SessionFilesDBYearColumn + " DESC").Limit(1)
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

// IsPlayerReady returns true if the player has a session_player_race entry with ready=true
func IsPlayerReady(sqlH database.SQLHelper, sessionID, userProfileID string) (bool, error) {
	query := database.SQ.Select(models.SessionPlayerRaceDBReadyColumn).
		From(models.SessionPlayerRaceDBTable).
		Where(sq.And{
			sq.Eq{models.SessionPlayerRaceDBSessionIDColumn: sessionID},
			sq.Eq{models.SessionPlayerRaceDBUserProfileIDColumn: userProfileID},
		})

	var ready bool
	if err := sqlH.Get(&ready, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// No session_player_race entry means player hasn't set up their race yet, so not ready
			return false, nil
		}
		return false, err
	}
	return ready, nil
}

// GetSessionInviteeIDs returns the list of user profile IDs who have pending invitations to the session
func GetSessionInviteeIDs(sqlH database.SQLHelper, sessionID string, log *zerolog.Logger) ([]string, error) {
	query := database.SQ.Select(models.InvitationDBUserProfileIDColumn).
		From(models.InvitationDBTable).
		Where(sq.Eq{models.InvitationDBSessionIDColumn: sessionID})

	rows, err := sqlH.Query(query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Err(err).Msg("failed to close rows")
		}
	}()

	var inviteeIDs []string
	for rows.Next() {
		var userProfileID string
		if err := rows.Scan(&userProfileID); err != nil {
			return nil, err
		}
		inviteeIDs = append(inviteeIDs, userProfileID)
	}
	return inviteeIDs, rows.Err()
}
