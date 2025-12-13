package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/fixtures"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
	"github.com/neper-stars/neper/sync"
)

func TestSessionsListHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// we only load sessions as gandalf.json contains users/perms that are not
	// tested here
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/mordor.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gandalf.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sarouman.json")

	handler := NewSessionsListHandler(testdb.DB)

	// here we do not test authorizations which are applied in the
	// layer just above the handler we are testing.
	// We can only test if the returned content is coherent with
	// our current user
	t.Run("gandalf_is_general_manager_and_sees_all", func(t *testing.T) {
		sessions, err := handler.handle(ctx, &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "gandalfID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		})
		require.NoError(t, err)
		require.Equal(t, 4, len(sessions)) // gondorID, isengardID, mordorID, shireID
	})

	t.Run("boromir_sees_public_sessions_and_his_memberships", func(t *testing.T) {
		sessions, err := handler.handle(ctx, &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		})
		require.NoError(t, err)
		// Boromir should see:
		// - gondorID: he is a member
		// - mordorID: it's public
		// - shireID: it's public (even though he's not a member)
		// But NOT isengardID: it's private and he's not a member
		require.Equal(t, 3, len(sessions))
		require.Equal(t, "gondorID", sessions[0].ID)
		require.Equal(t, "mordorID", sessions[1].ID)
		require.Equal(t, "shireID", sessions[2].ID)
	})

	t.Run("merry_sees_only_public_sessions_and_his_memberships", func(t *testing.T) {
		sessions, err := handler.handle(ctx, &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		})
		require.NoError(t, err)
		// Merry should see:
		// - gondorID: it's public
		// - mordorID: it's public
		// - shireID: it's public
		// But NOT isengardID: it's private and he's not a member
		require.Equal(t, 3, len(sessions))
		require.Equal(t, "gondorID", sessions[0].ID)
		require.Equal(t, "mordorID", sessions[1].ID)
		require.Equal(t, "shireID", sessions[2].ID)
	})

	t.Run("sarouman_sees_public_sessions_and_his_memberships", func(t *testing.T) {
		sessions, err := handler.handle(ctx, &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "saroumanID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		})
		require.NoError(t, err)
		// Sarouman should see:
		// - gondorID: it's public
		// - isengardID: it's private, but he is a member
		// - mordorID: it's public
		// - shireID: it's public
		// in this order as they are sorted by id
		require.Equal(t, 4, len(sessions))
		require.Equal(t, "gondorID", sessions[0].ID)
		require.Equal(t, "isengardID", sessions[1].ID)
		isengardSession := sessions[1]
		require.Equal(t, true, isengardSession.Private)
		// only sarouman is manager of isengard
		require.Equal(t, 1, len(isengardSession.Managers))
		require.Equal(t, "saroumanID", isengardSession.Managers[0])
		require.Equal(t, "mordorID", sessions[2].ID)
		require.Equal(t, "shireID", sessions[3].ID)
	})

	t.Run("started_field_is_returned_correctly", func(t *testing.T) {
		sessions, err := handler.handle(ctx, &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "gandalfID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		})
		require.NoError(t, err)

		// Find mordor session and verify the started field is true
		var mordorSession *models.Session
		for _, s := range sessions {
			if s.ID == "mordorID" {
				mordorSession = s
				break
			}
		}
		require.NotNil(t, mordorSession, "mordorID should be in the list")
		require.True(t, mordorSession.Started, "Started field should be true for mordorID")
		require.True(t, mordorSession.RulesIsSet, "RulesIsSet field should be true for mordorID")

		// Also verify that non-started sessions have started=false
		var gondorSession *models.Session
		for _, s := range sessions {
			if s.ID == "gondorID" {
				gondorSession = s
				break
			}
		}
		require.NotNil(t, gondorSession, "gondorID should be in the list")
		require.False(t, gondorSession.Started, "Started field should be false for gondorID")
	})

	t.Run("rules_is_set_is_true_after_creating_rules", func(t *testing.T) {
		rulesHandler := NewRulesCreateHandler(&log, testdb.DB, nil)

		gandalfPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "gandalfID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Create rules for gondorID session (gandalf is manager)
		rulesParams := operations.RulesCreateParams{
			SessionID: "gondorID",
			Ruleset: &models.Ruleset{
				UniverseSize:     3,
				Density:          2,
				StartingDistance: 3,
				RandomSeed:       12345,
			},
		}
		_, err := rulesHandler.handle(ctx, rulesParams, gandalfPrincipal)
		require.NoError(t, err)

		// Now list sessions and verify RulesIsSet is true for gondorID
		sessions, err := handler.handle(ctx, &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "gandalfID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		})
		require.NoError(t, err)

		// Find gondorID session and verify RulesIsSet is true
		var gondorSession *models.Session
		for _, s := range sessions {
			if s.ID == "gondorID" {
				gondorSession = s
				break
			}
		}
		require.NotNil(t, gondorSession, "gondorID session should be in the list")
		require.True(t, gondorSession.RulesIsSet, "RulesIsSet should be true after creating rules")
	})
}
