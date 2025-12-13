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

func TestRulesCreateHandler_SetsRulesIsSet(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	// Load session and user fixtures
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gandalf.json")

	rulesHandler := NewRulesCreateHandler(&log, testdb.DB, nil)
	sessionHandler := NewSessionReadHandler(&log, testdb.DB)

	gandalfPrincipal := &models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "gandalfID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		IsGlobalManager: false,
	}

	t.Run("rules_is_set_becomes_true_after_creating_rules", func(t *testing.T) {
		sessionID := "gondorID"

		// First verify the session has rules_is_set = false
		sessionParams := operations.SessionReadParams{
			SessionID: sessionID,
		}
		session, err := sessionHandler.handle(ctx, sessionParams, gandalfPrincipal)
		require.NoError(t, err)
		require.False(t, session.RulesIsSet, "rules_is_set should be false before creating rules")

		// Create rules for the session
		rulesParams := operations.RulesCreateParams{
			SessionID: sessionID,
			Ruleset: &models.Ruleset{
				UniverseSize:     3,
				Density:          2,
				StartingDistance: 3,
				RandomSeed:       12345,
			},
		}
		_, err = rulesHandler.handle(ctx, rulesParams, gandalfPrincipal)
		require.NoError(t, err)

		// Verify the session now has rules_is_set = true
		session, err = sessionHandler.handle(ctx, sessionParams, gandalfPrincipal)
		require.NoError(t, err)
		require.True(t, session.RulesIsSet, "rules_is_set should be true after creating rules")
	})
}

func TestRulesCreateHandler_UpdatesExistingRules(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	// Load session and user fixtures
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gandalf.json")

	rulesHandler := NewRulesCreateHandler(&log, testdb.DB, nil)
	rulesReadHandler := NewRulesReadHandler(&log, testdb.DB)

	gandalfPrincipal := &models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "gandalfID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		IsGlobalManager: false,
	}

	sessionID := "gondorID"

	t.Run("updating_rules_does_not_create_duplicate", func(t *testing.T) {
		// Create initial rules
		rulesParams := operations.RulesCreateParams{
			SessionID: sessionID,
			Ruleset: &models.Ruleset{
				UniverseSize:     3,
				Density:          2,
				StartingDistance: 3,
				RandomSeed:       12345,
			},
		}
		firstRuleset, err := rulesHandler.handle(ctx, rulesParams, gandalfPrincipal)
		require.NoError(t, err)
		require.Equal(t, int64(3), firstRuleset.UniverseSize)

		// Count rulesets for this session
		var count int
		err = testdb.Get(&count, "SELECT COUNT(*) FROM ruleset WHERE session_id = $1", sessionID)
		require.NoError(t, err)
		require.Equal(t, 1, count, "should have exactly 1 ruleset after first creation")

		// Update rules with different values (keeping valid density range 0-3)
		rulesParams.Ruleset.UniverseSize = 5
		rulesParams.Ruleset.Density = 3
		secondRuleset, err := rulesHandler.handle(ctx, rulesParams, gandalfPrincipal)
		require.NoError(t, err)
		require.Equal(t, int64(5), secondRuleset.UniverseSize)
		require.Equal(t, int64(3), secondRuleset.Density)

		// Count rulesets again - should still be 1
		err = testdb.Get(&count, "SELECT COUNT(*) FROM ruleset WHERE session_id = $1", sessionID)
		require.NoError(t, err)
		require.Equal(t, 1, count, "should still have exactly 1 ruleset after update")

		// Read the rules back and verify the update took effect
		readParams := operations.RulesReadParams{
			SessionID: sessionID,
		}
		readRuleset, err := rulesReadHandler.handle(ctx, readParams, gandalfPrincipal)
		require.NoError(t, err)
		require.Equal(t, int64(5), readRuleset.UniverseSize)
		require.Equal(t, int64(3), readRuleset.Density)
	})
}

func TestRulesCreateHandler_FailsForStartedSession(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	// Load fixture with started session
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/mordor.json")

	rulesHandler := NewRulesCreateHandler(&log, testdb.DB, nil)

	managerPrincipal := &models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "managerID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		IsGlobalManager: false,
	}

	t.Run("cannot_set_rules_for_started_session", func(t *testing.T) {
		rulesParams := operations.RulesCreateParams{
			SessionID: "startedSessionID",
			Ruleset: &models.Ruleset{
				UniverseSize:     3,
				Density:          2,
				StartingDistance: 3,
				RandomSeed:       12345,
			},
		}
		_, err := rulesHandler.handle(ctx, rulesParams, managerPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrPreconditionFailed), "should return precondition failed error")
	})
}
