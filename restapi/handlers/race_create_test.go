package handlers

import (
	"context"
	"encoding/base64"
	"io"
	"os"
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

func TestRaceCreateHandler(t *testing.T) {
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

	handler := NewRaceCreateHandler(&log, testdb.DB, nil)

	t.Run("boromir_creates_humans", func(t *testing.T) {
		rf, err := os.Open("fixtures/humans.r1")
		require.NoError(t, err)

		data, err := io.ReadAll(rf)
		require.NoError(t, err)

		dst := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
		base64.StdEncoding.Encode(dst, data)

		boromirID := "boromirID"

		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   boromirID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		race := models.Race{
			Data: string(dst),
		}

		params := operations.RaceCreateParams{
			UserProfileID: boromirID,
			Race:          &race,
		}

		returnedRace, err := handler.handle(
			ctx,
			params,
			&boromirPrincipal,
		)

		require.NoError(t, err)
		// Humans is a string that was set inside the race file during its creation
		// this proves our parser can successfully read race files
		require.Equal(t, "Humans", returnedRace.NamePlural)
		require.Equal(t, "Human", returnedRace.NameSingular)
		require.Equal(t, boromirID, returnedRace.UserID)
	})
}
