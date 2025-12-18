package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestSessionReadHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// we only load sessions as gandalf.json contains users/perms that are not
	// tested here
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")

	handler := NewSessionReadHandler(&log, testdb.DB)

	t.Run("empty Gondor", func(t *testing.T) {
		p := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.SessionReadParams{
			SessionID: "gondorID",
		}
		session, err := handler.handle(ctx, params, p)

		require.NoError(t, err)
		require.Equal(t, "Gondor", session.Name)
		require.Equal(t, 0, len(session.Members))
		require.Equal(t, 0, len(session.Managers))
	})

	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gandalf.json")

	t.Run("Gondor with Gandalf as manager", func(t *testing.T) {
		p := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.SessionReadParams{
			SessionID: "gondorID",
		}
		session, err := handler.handle(ctx, params, p)
		require.NoError(t, err)
		require.Equal(t, "Gondor", session.Name)
		require.Equal(t, 0, len(session.Members))
		require.Equal(t, 1, len(session.Managers))
		require.Equal(t, "gandalfID", session.Managers[0])
	})

	t.Run("isengard_is_private_and_merry_is_not_a_member", func(t *testing.T) {
		p := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.SessionReadParams{
			SessionID: "isengardID",
		}
		_, err := handler.handle(ctx, params, p)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden))
	})

	// Load merry_nosession to get a user without any session membership
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")

	t.Run("invited_user_can_see_private_session", func(t *testing.T) {
		// Create an invitation for merry to isengard (private session)
		_, err := testdb.Exec(`INSERT INTO invitation (id, session_id, user_profile_id, inviter_id) VALUES ('invitationID', 'isengardID', 'merryID', 'gandalfID')`)
		require.NoError(t, err)

		p := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.SessionReadParams{
			SessionID: "isengardID",
		}
		session, err := handler.handle(ctx, params, p)
		require.NoError(t, err)
		require.Equal(t, "Isengard", session.Name)
	})
}

func TestSessionReadHandler_PlayersReady(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	handler := NewSessionReadHandler(&log, testdb.DB)

	t.Run("players_ready_state_is_returned", func(t *testing.T) {
		// Load fixture with players that have ready: true
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum.json")

		p := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.SessionReadParams{
			SessionID: "merryvsgollumID",
		}
		session, err := handler.handle(ctx, params, p)
		require.NoError(t, err)
		require.Equal(t, "MerryVSGollum", session.Name)
		require.Equal(t, 2, len(session.Players))

		// First player is merry, ready
		require.Equal(t, "merryID", session.Players[0].UserProfileID)
		require.True(t, session.Players[0].Ready)

		// Second player is gollum, ready
		require.Equal(t, "gollumID", session.Players[1].UserProfileID)
		require.True(t, session.Players[1].Ready)
	})

	t.Run("players_not_ready_state_is_returned", func(t *testing.T) {
		// Load fixture with players that have ready: false
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_not_ready.json")

		p := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.SessionReadParams{
			SessionID: "merryvsgollumID",
		}
		session, err := handler.handle(ctx, params, p)
		require.NoError(t, err)
		require.Equal(t, "MerryVSGollum", session.Name)
		require.Equal(t, 2, len(session.Players))

		// First player is merry, not ready
		require.Equal(t, "merryID", session.Players[0].UserProfileID)
		require.False(t, session.Players[0].Ready)

		// Second player is gollum, not ready
		require.Equal(t, "gollumID", session.Players[1].UserProfileID)
		require.False(t, session.Players[1].Ready)
	})
}
