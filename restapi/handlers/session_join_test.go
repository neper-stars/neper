package handlers

import (
	"context"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/stretchr/testify/require"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/fixtures"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
	"github.com/neper-stars/neper/sync"
)

func TestSessionJoinHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	// Load fixtures
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sarouman.json")

	joinHandler := NewSessionJoinHandler(&log, testdb.DB, nil)

	merryPrincipal := &models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "merryID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		IsGlobalManager: false,
	}

	t.Run("user_can_join_public_session", func(t *testing.T) {
		joinParams := operations.SessionJoinParams{
			SessionID: "gondorID", // gondor is public
		}
		session, err := joinHandler.handle(ctx, joinParams, merryPrincipal)
		require.NoError(t, err)
		require.NotNil(t, session)
		require.Equal(t, "gondorID", session.ID)

		// Verify merry is now a member
		found := false
		for _, member := range session.Members {
			if member == "merryID" {
				found = true
				break
			}
		}
		require.True(t, found, "merry should be in the members list after joining")
	})

	t.Run("user_cannot_join_private_session_without_invitation", func(t *testing.T) {
		joinParams := operations.SessionJoinParams{
			SessionID: "isengardID", // isengard is private
		}
		_, err := joinHandler.handle(ctx, joinParams, merryPrincipal)
		require.Error(t, err)
		require.ErrorIs(t, err, errs.ErrForbidden)
	})

	t.Run("invited_user_can_join_private_session", func(t *testing.T) {
		// Create an invitation for merry to isengard (private session)
		_, err := testdb.Exec(`INSERT INTO invitation (id, session_id, user_profile_id, inviter_id) VALUES ('inviteMerryToIsengard', 'isengardID', 'merryID', 'saroumanID')`)
		require.NoError(t, err)

		joinParams := operations.SessionJoinParams{
			SessionID: "isengardID",
		}
		session, err := joinHandler.handle(ctx, joinParams, merryPrincipal)
		require.NoError(t, err)
		require.NotNil(t, session)
		require.Equal(t, "isengardID", session.ID)

		// Verify merry is now a member
		found := false
		for _, member := range session.Members {
			if member == "merryID" {
				found = true
				break
			}
		}
		require.True(t, found, "merry should be in the members list after joining")

		// Verify invitation was deleted
		var count int
		err = testdb.Get(&count, `SELECT COUNT(*) FROM invitation WHERE id = 'inviteMerryToIsengard'`)
		require.NoError(t, err)
		require.Equal(t, 0, count, "invitation should be deleted after joining")
	})

	t.Run("user_cannot_join_same_session_twice", func(t *testing.T) {
		// Merry already joined gondor in the first test
		joinParams := operations.SessionJoinParams{
			SessionID: "gondorID",
		}
		_, err := joinHandler.handle(ctx, joinParams, merryPrincipal)
		require.Error(t, err)
		require.ErrorIs(t, err, errs.ErrConflict)
	})

	t.Run("user_cannot_join_nonexistent_session", func(t *testing.T) {
		joinParams := operations.SessionJoinParams{
			SessionID: "nonexistentID",
		}
		_, err := joinHandler.handle(ctx, joinParams, merryPrincipal)
		require.Error(t, err)
		require.ErrorIs(t, err, errs.ErrForbidden)
	})

	t.Run("global_manager_can_join_private_session", func(t *testing.T) {
		// Create a new user who is a global manager
		globalManagerPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "saroumanID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		joinParams := operations.SessionJoinParams{
			SessionID: "shireID", // shire is public, but testing that global manager works
		}
		session, err := joinHandler.handle(ctx, joinParams, globalManagerPrincipal)
		require.NoError(t, err)
		require.NotNil(t, session)
		require.Equal(t, "shireID", session.ID)
	})
}

func TestSessionJoinHandler_DeletesInvitation(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	// Load fixtures
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")

	joinHandler := NewSessionJoinHandler(&log, testdb.DB, nil)
	invitationCreateHandler := NewInvitationCreateHandler(&log, testdb.DB, nil)

	boromirPrincipal := &models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "boromirID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		IsGlobalManager: false,
	}

	merryPrincipal := &models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "merryID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		IsGlobalManager: false,
	}

	t.Run("joining_session_deletes_existing_invitation", func(t *testing.T) {
		// Boromir (manager of gondor) invites Merry to gondor
		inviteParams := operations.InvitationCreateParams{
			SessionID: "gondorID",
			Invitation: &models.Invitation{
				SessionID:     "gondorID",
				UserProfileID: "merryID",
			},
		}
		invitation, err := invitationCreateHandler.handle(ctx, inviteParams, boromirPrincipal)
		require.NoError(t, err)
		require.NotNil(t, invitation)
		invitationID := invitation.ID

		// Verify invitation exists
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var count int
		err = sqlH.Get(&count, database.SQ.Select("COUNT(*)").From(models.InvitationDBTable).Where(sq.Eq{models.InvitationDBIDColumn: invitationID}))
		require.NoError(t, err)
		require.Equal(t, 1, count, "invitation should exist before join")

		// Merry joins gondor (which is public)
		joinParams := operations.SessionJoinParams{
			SessionID: "gondorID",
		}
		session, err := joinHandler.handle(ctx, joinParams, merryPrincipal)
		require.NoError(t, err)
		require.NotNil(t, session)

		// Verify invitation was deleted
		err = sqlH.Get(&count, database.SQ.Select("COUNT(*)").From(models.InvitationDBTable).Where(sq.Eq{models.InvitationDBIDColumn: invitationID}))
		require.NoError(t, err)
		require.Equal(t, 0, count, "invitation should be deleted after join")
	})
}
