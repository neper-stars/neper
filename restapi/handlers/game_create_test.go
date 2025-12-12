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
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
	"github.com/neper-stars/neper/sync"
)

func getTestPrincipal(userID string, globalManager bool) models.Principal {
	return models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   userID,
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		IsGlobalManager: globalManager,
	}
}

func TestGameCreateHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// load some predefined session with 2 players, that already have set up
	// their races
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum.json")
	// add a ruleset to our session
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_vs_gollum_ruleset.json")

	runner := stars.GetTestStarsRunner(t, &log)
	createHandler := NewGameCreateHandler(&log, testdb.DB, runner, nil)

	t.Run("merry_generates_the_first_turn", func(t *testing.T) {
		sessionID := "merryvsgollumID"
		merryID := "merryID"
		merryPrincipal := getTestPrincipal(merryID, false)

		params := operations.NewGameCreateParams()
		params.SessionID = sessionID

		// the handler returns all the game files
		returnedFiles, err := createHandler.handle(ctx, params, &merryPrincipal)
		require.NoError(t, err)
		require.Equal(t, sessionID, returnedFiles.SessionID)
		require.Equal(t, 2, len(returnedFiles.Turns))
		require.Equal(t, 0, len(returnedFiles.Orders))
		require.True(t, len(returnedFiles.HostFile) > 0)
		print(returnedFiles.HostFile)

		// Verify the session is marked as started
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var sessionDB models.SessionDB
		require.NoError(t, sqlH.GetByPKey(&sessionDB, sessionID))
		require.True(t, sessionDB.Started, "session should be marked as started after game creation")

		var rawData []byte
		_, err = stars.B64Decode(returnedFiles.HostFile)
		require.NoError(t, err)
		raceNames, err := stars.RaceNamesFromHostFile(rawData)
		require.NoError(t, err)
		for i := range raceNames {
			switch i {
			case 0:
				// first player plays with hobbits
				require.Equal(t, "Hobbits", raceNames[i])
			case 1:
				// second player plays with halflings
				require.Equal(t, "Halflings", raceNames[i])
			default:
				t.Fail()
			}
		}
	})
}

func TestGameCreateHandler_PlayersNotReady(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// Load fixture without ready flags set
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_not_ready.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_vs_gollum_ruleset.json")

	runner := stars.GetTestStarsRunner(t, &log)
	createHandler := NewGameCreateHandler(&log, testdb.DB, runner, nil)

	t.Run("cannot_start_game_when_players_not_ready", func(t *testing.T) {
		sessionID := "merryvsgollumID"
		merryID := "merryID"
		merryPrincipal := getTestPrincipal(merryID, false)

		params := operations.NewGameCreateParams()
		params.SessionID = sessionID

		_, err := createHandler.handle(ctx, params, &merryPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrPreconditionFailed), "should get precondition failed when players not ready")
	})
}
