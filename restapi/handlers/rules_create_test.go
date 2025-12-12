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

	rulesHandler := NewRulesCreateHandler(&log, testdb.DB)
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
