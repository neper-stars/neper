package handlers

import (
	"archive/zip"
	"bytes"
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

func TestHistoricBackupHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	// Load session with players, races, and turn files
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")

	handler := NewHistoricBackupHandler(&log, testdb.DB)

	t.Run("session_manager_gets_all_files", func(t *testing.T) {
		// Merry is the session manager
		merryPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.HistoricBackupParams{
			SessionID: "merryvsgollumID",
		}

		result, err := handler.handle(ctx, params, merryPrincipal)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.ZipData)

		// Read and validate the ZIP contents
		zipReader, err := zip.NewReader(bytes.NewReader(result.ZipData.Bytes()), int64(result.ZipData.Len()))
		require.NoError(t, err)

		// Manager should get all files
		fileNames := make(map[string]bool)
		for _, f := range zipReader.File {
			fileNames[f.Name] = true
		}

		// Manager gets host file
		require.True(t, fileNames["game-2400.hst"], "manager should get host file")
		// Manager gets universe file
		require.True(t, fileNames["game-2400.xy"], "manager should get universe file")
		// Manager gets all turn files
		require.True(t, fileNames["game-2400.m1"], "manager should get player 1 turn file")
		require.True(t, fileNames["game-2400.m2"], "manager should get player 2 turn file")
		// Manager gets all race files
		require.True(t, fileNames["game.r1"], "manager should get player 1 race file")
		require.True(t, fileNames["game.r2"], "manager should get player 2 race file")
	})

	t.Run("global_manager_gets_all_files", func(t *testing.T) {
		globalManagerPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "someOtherUserID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}
		params := operations.HistoricBackupParams{
			SessionID: "merryvsgollumID",
		}

		result, err := handler.handle(ctx, params, globalManagerPrincipal)
		require.NoError(t, err)
		require.NotNil(t, result)

		zipReader, err := zip.NewReader(bytes.NewReader(result.ZipData.Bytes()), int64(result.ZipData.Len()))
		require.NoError(t, err)

		fileNames := make(map[string]bool)
		for _, f := range zipReader.File {
			fileNames[f.Name] = true
		}

		// Global manager gets all files just like session manager
		require.True(t, fileNames["game-2400.hst"], "global manager should get host file")
		require.True(t, fileNames["game-2400.xy"], "global manager should get universe file")
		require.True(t, fileNames["game-2400.m1"], "global manager should get player 1 turn file")
		require.True(t, fileNames["game-2400.m2"], "global manager should get player 2 turn file")
	})

	t.Run("regular_player_gets_only_own_files", func(t *testing.T) {
		// Gollum is a regular member (player 2 / order 1)
		gollumPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "gollumID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.HistoricBackupParams{
			SessionID: "merryvsgollumID",
		}

		result, err := handler.handle(ctx, params, gollumPrincipal)
		require.NoError(t, err)
		require.NotNil(t, result)

		zipReader, err := zip.NewReader(bytes.NewReader(result.ZipData.Bytes()), int64(result.ZipData.Len()))
		require.NoError(t, err)

		fileNames := make(map[string]bool)
		for _, f := range zipReader.File {
			fileNames[f.Name] = true
		}

		// Regular player does NOT get host file
		require.False(t, fileNames["game-2400.hst"], "regular player should NOT get host file")
		// Regular player gets universe file
		require.True(t, fileNames["game-2400.xy"], "regular player should get universe file")
		// Regular player gets only their own turn file (player 2 = .m2)
		require.False(t, fileNames["game-2400.m1"], "regular player should NOT get other player's turn file")
		require.True(t, fileNames["game-2400.m2"], "regular player should get their own turn file")
		// Regular player gets only their own race file (player 2 = .r2)
		require.False(t, fileNames["game.r1"], "regular player should NOT get other player's race file")
		require.True(t, fileNames["game.r2"], "regular player should get their own race file")
	})

	t.Run("non_member_gets_forbidden", func(t *testing.T) {
		nonMemberPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "randomUserID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.HistoricBackupParams{
			SessionID: "merryvsgollumID",
		}

		_, err := handler.handle(ctx, params, nonMemberPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden), "non-member should get forbidden error")
	})

	t.Run("not_started_session_returns_precondition_failed", func(t *testing.T) {
		// Load a session that is not started
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")

		globalManagerPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "someUserID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}
		params := operations.HistoricBackupParams{
			SessionID: "gondorID", // Not started session
		}

		_, err := handler.handle(ctx, params, globalManagerPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrPreconditionFailed), "not started session should return precondition failed")
	})

	t.Run("nonexistent_session_returns_not_found", func(t *testing.T) {
		globalManagerPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "someUserID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}
		params := operations.HistoricBackupParams{
			SessionID: "nonexistentSessionID",
		}

		_, err := handler.handle(ctx, params, globalManagerPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrNotFound), "nonexistent session should return not found")
	})
}
