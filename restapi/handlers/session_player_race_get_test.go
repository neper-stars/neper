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

func TestSessionPlayerRaceGetHandler(t *testing.T) {
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

	createHandler := NewSessionPlayerRaceCreateHandler(&log, testdb.DB, nil)
	getHandler := NewSessionPlayerRaceGetHandler(testdb.DB)

	boromirPrincipal := &models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "boromirID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		IsGlobalManager: false,
	}

	t.Run("get_race_before_setting_one_returns_not_found", func(t *testing.T) {
		getParams := operations.SessionPlayerRaceGetParams{
			SessionID: "gondorID",
		}
		_, err := getHandler.handle(ctx, getParams, boromirPrincipal)
		require.Error(t, err)
		require.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("get_race_after_setting_one_returns_the_race", func(t *testing.T) {
		// First, set a race for boromir in gondor session
		createParams := operations.SessionPlayerRaceCreateParams{
			SessionID: "gondorID",
			SessionPlayerRace: &models.SessionPlayerRace{
				RaceID: "humansID",
			},
		}
		_, err := createHandler.handle(ctx, createParams, boromirPrincipal)
		require.NoError(t, err)

		// Now get the race
		getParams := operations.SessionPlayerRaceGetParams{
			SessionID: "gondorID",
		}
		race, err := getHandler.handle(ctx, getParams, boromirPrincipal)
		require.NoError(t, err)
		require.NotNil(t, race)
		require.Equal(t, "humansID", race.ID)
		require.Equal(t, "Humans", race.NamePlural)
		require.Equal(t, "Human", race.NameSingular)
		require.NotEmpty(t, race.Data, "race data should be returned")
	})

	t.Run("get_race_for_different_session_returns_not_found", func(t *testing.T) {
		// boromir has set a race for gondorID but not for shireID
		getParams := operations.SessionPlayerRaceGetParams{
			SessionID: "shireID",
		}
		_, err := getHandler.handle(ctx, getParams, boromirPrincipal)
		require.Error(t, err)
		require.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("different_player_gets_their_own_race", func(t *testing.T) {
		// Add merry to gondor session
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
						"state": "active",
						"is_manager": false,
						"session_list": [{"session_id": "gondorID", "is_manager": false}]
					}
				}
			]
		}`)
		fixtures.LoadFixtureJSON(t, syncWorker, addMerryToGondor)

		merryPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Merry sets hobbits as his race
		createParams := operations.SessionPlayerRaceCreateParams{
			SessionID: "gondorID",
			SessionPlayerRace: &models.SessionPlayerRace{
				RaceID: "hobbitsID",
			},
		}
		_, err := createHandler.handle(ctx, createParams, merryPrincipal)
		require.NoError(t, err)

		// Merry gets his race - should be hobbits, not humans
		getParams := operations.SessionPlayerRaceGetParams{
			SessionID: "gondorID",
		}
		race, err := getHandler.handle(ctx, getParams, merryPrincipal)
		require.NoError(t, err)
		require.NotNil(t, race)
		require.Equal(t, "hobbitsID", race.ID)
		require.Equal(t, "Hobbits", race.NamePlural)

		// Boromir still gets humans
		boromirRace, err := getHandler.handle(ctx, getParams, boromirPrincipal)
		require.NoError(t, err)
		require.Equal(t, "humansID", boromirRace.ID)
	})
}
