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

	createHandler := NewSessionPlayerRaceCreateHandler(&log, testdb.DB)

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
