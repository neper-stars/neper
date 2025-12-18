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

func TestRulesReadHandler_PublicSessionAccess(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	// Load fixtures:
	// - sessions.json has gondorID (public) and isengardID (private)
	// - gandalf.json has gandalf who is a member of gondorID
	// - merry_nosession.json has merry who is not a member of any session
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gandalf.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")

	rulesCreateHandler := NewRulesCreateHandler(&log, testdb.DB, nil)
	rulesReadHandler := NewRulesReadHandler(&log, testdb.DB)

	gandalfPrincipal := &models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "gandalfID",
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

	// First, create rules for both public and private sessions using gandalf (who is a manager)
	publicSessionID := "gondorID"
	privateSessionID := "isengardID"

	// Create rules for public session
	rulesParams := operations.RulesCreateParams{
		SessionID: publicSessionID,
		Ruleset: &models.Ruleset{
			UniverseSize:     3,
			Density:          2,
			StartingDistance: 3,
			RandomSeed:       12345,
		},
	}
	_, err = rulesCreateHandler.handle(ctx, rulesParams, gandalfPrincipal)
	require.NoError(t, err)

	// For private session, we need to make gandalf a manager first
	// Add gandalf as manager of isengard
	_, err = testdb.Exec(`INSERT INTO user_profile_session_rel (user_profile_id, session_id, is_manager) VALUES ('gandalfID', 'isengardID', true)`)
	require.NoError(t, err)

	// Create rules for private session
	rulesParams.SessionID = privateSessionID
	_, err = rulesCreateHandler.handle(ctx, rulesParams, gandalfPrincipal)
	require.NoError(t, err)

	t.Run("non_member_can_read_rules_of_public_session", func(t *testing.T) {
		readParams := operations.RulesReadParams{
			SessionID: publicSessionID,
		}
		ruleset, err := rulesReadHandler.handle(ctx, readParams, merryPrincipal)
		require.NoError(t, err)
		require.NotNil(t, ruleset)
		require.Equal(t, int64(3), ruleset.UniverseSize)
	})

	t.Run("non_member_cannot_read_rules_of_private_session", func(t *testing.T) {
		readParams := operations.RulesReadParams{
			SessionID: privateSessionID,
		}
		_, err := rulesReadHandler.handle(ctx, readParams, merryPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden), "should return forbidden error for non-member accessing private session")
	})

	t.Run("member_can_read_rules_of_private_session", func(t *testing.T) {
		readParams := operations.RulesReadParams{
			SessionID: privateSessionID,
		}
		ruleset, err := rulesReadHandler.handle(ctx, readParams, gandalfPrincipal)
		require.NoError(t, err)
		require.NotNil(t, ruleset)
	})
}
