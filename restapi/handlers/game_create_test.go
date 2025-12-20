package handlers

import (
	"context"
	"errors"
	"os"
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

		// the handler returns the requesting player's turn files (not all session files)
		returnedFiles, err := createHandler.handle(ctx, params, &merryPrincipal)
		require.NoError(t, err)
		require.Equal(t, sessionID, returnedFiles.SessionID)
		require.Equal(t, int64(2400), returnedFiles.Year)
		// Player gets their own turn file
		require.NotNil(t, returnedFiles.Turn)
		require.True(t, len(returnedFiles.Turn.Turn) > 0, "player should receive their turn file")
		require.True(t, len(returnedFiles.Turn.Universe) > 0, "player should receive the universe file")

		// Verify the session is marked as started
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var sessionDB models.SessionDB
		require.NoError(t, sqlH.GetByPKey(&sessionDB, sessionID))
		require.True(t, sessionDB.Started, "session should be marked as started after game creation")

		// Verify host file is stored in database (but NOT returned to client)
		var sessionFilesDB models.SessionFilesDB
		require.NoError(t, sqlH.GetBy(&sessionFilesDB, "session_id", sessionID))
		require.True(t, len(sessionFilesDB.HostFile) > 0, "host file should be stored in database")

		// Verify we can parse race names from the stored host file
		rawData, err := stars.B64Decode(sessionFilesDB.HostFile)
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
				// Note: The binary race data has "Halfings" (typo in the original race file)
				require.Equal(t, "Halfings", raceNames[i])
			default:
				t.Fail()
			}
		}

		// Verify session directory was cleaned up after game creation
		sessionDir := runner.SessionDir(sessionID)
		_, err = os.Stat(sessionDir)
		require.True(t, os.IsNotExist(err), "session directory should be cleaned up after game creation")
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
