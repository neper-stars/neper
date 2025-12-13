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
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
	"github.com/neper-stars/neper/sync"
)

func TestSessionPlayerRaceCreateHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// load some sessions because our players are members of them
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	// humans are owned by Boromir
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	// hobbits are owned by Merry
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")
	// load our races
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/races.json")

	createHandler := NewSessionPlayerRaceCreateHandler(&log, testdb.DB, nil)

	t.Run("boromir_tries_to_register_with_a_race_he_does_not_own", func(t *testing.T) {
		sprCreateParams := operations.SessionPlayerRaceCreateParams{
			SessionID: "gondorID",
			SessionPlayerRace: &models.SessionPlayerRace{
				RaceID: "hobbitsID", // <-- boromir has no right to use hobbits
			},
		}
		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		_, err := createHandler.handle(ctx, sprCreateParams, &boromirPrincipal)
		require.Error(t, err)
		require.Equal(t, errs.ErrForbidden, err) // <-- so boromir gets a nice forbidden error in his human face
	})

	t.Run("boromir_will_play_with_humans_in_gondor_session", func(t *testing.T) {
		sprCreateParams := operations.SessionPlayerRaceCreateParams{
			SessionID: "gondorID",
			SessionPlayerRace: &models.SessionPlayerRace{
				RaceID: "humansID",
			},
		}
		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		spr, err := createHandler.handle(ctx, sprCreateParams, &boromirPrincipal)
		require.NoError(t, err)
		require.Equal(t, spr.RaceID, "humansID")
	})
}

func TestSessionPlayerRaceCreateHandler_PlayerOrderAssignment(t *testing.T) {
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
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/races.json")

	// Override Merry to be a member of the gondor session so he can register
	addMerryToGondor := []byte(`{
		"changes": [
			{
				"operation": "create",
				"data": {
					"__type__": "user_profile",
					"id": "merryID",
					"nickname": "Merry",
					"email": "merry@shire.com",
					"api_key": "apikeyMerry",
					"is_active": true,
					"is_manager": false,
					"session_list": [{"session_id": "gondorID", "is_manager": false}]
				}
			}
		]
	}`)
	fixtures.LoadFixtureJSON(t, syncWorker, addMerryToGondor)

	createHandler := NewSessionPlayerRaceCreateHandler(&log, testdb.DB, nil)

	t.Run("multiple_players_get_sequential_player_orders", func(t *testing.T) {
		sessionID := "gondorID"

		// Boromir registers first with humans
		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		sprBoromir, err := createHandler.handle(ctx, operations.SessionPlayerRaceCreateParams{
			SessionID: sessionID,
			SessionPlayerRace: &models.SessionPlayerRace{
				RaceID: "humansID",
			},
		}, &boromirPrincipal)
		require.NoError(t, err)
		require.Equal(t, int64(0), sprBoromir.PlayerOrder, "first player should get player_order 0")

		// Merry registers second with hobbits
		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		sprMerry, err := createHandler.handle(ctx, operations.SessionPlayerRaceCreateParams{
			SessionID: sessionID,
			SessionPlayerRace: &models.SessionPlayerRace{
				RaceID: "hobbitsID",
			},
		}, &merryPrincipal)
		require.NoError(t, err)
		require.Equal(t, int64(1), sprMerry.PlayerOrder, "second player should get player_order 1")

		// Verify all players have correct orders (they should not collide)
		require.NotEqual(t, sprBoromir.PlayerOrder, sprMerry.PlayerOrder,
			"players should have different player_order values")
	})
}
