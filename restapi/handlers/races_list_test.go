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

func TestRacesListHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// load some sessions because our players are members of them
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	// load some players
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gandalf.json")
	// humans are owned by Boromir
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	// hobbits are owned by Merry
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")
	// load races after players hobbits for Merry and Humans for Boromir
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/races.json")

	handler := NewRacesListHandler(testdb.DB)

	// here we do not test authorizations which are applied in the
	// layer just above the handler we are testing.
	// We can only test if the returned content is coherent with
	// our current user
	t.Run("gandalf_is_general_manager_and_sees_all", func(t *testing.T) {
		gandalfPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "gandalfID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		params := operations.RacesListParams{
			UserProfileID: "merryID",
		}

		races, err := handler.handle(ctx, params, &gandalfPrincipal)
		require.NoError(t, err)
		require.Equal(t, 2, len(races), "Gandalf is general manager and should see merry races")
		// order by ID, so halflings are before hobbits :p
		require.Equal(t, "Halflings", races[0].NamePlural)
		require.Equal(t, "Hobbits", races[1].NamePlural)
	})

	t.Run("boromir_owns_only_humans", func(t *testing.T) {
		boromirID := "boromirID"
		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   boromirID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.RacesListParams{
			UserProfileID: boromirID,
		}
		races, err := handler.handle(ctx, params, &boromirPrincipal)
		require.NoError(t, err)
		require.Equal(t, 1, len(races))
		require.Equal(t, "Humans", races[0].NamePlural)
	})

	t.Run("merry_owns_only_hobbits_and_halflings", func(t *testing.T) {
		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		params := operations.RacesListParams{
			UserProfileID: "boromirID", // <-- try to see a race we don't own
		}

		_, err := handler.handle(ctx, params, &merryPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden))
	})
}
