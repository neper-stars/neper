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

func TestSessionFilesGetHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	// Load a session with turn files
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

	handler := NewSessionFilesGetHandler(&log, testdb.DB)

	t.Run("session_manager_can_get_all_files", func(t *testing.T) {
		// Merry is the session manager
		merryPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.SessionFilesGetParams{
			SessionID: "merryvsgollumID",
		}
		sessionFiles, err := handler.handle(ctx, params, merryPrincipal)
		require.NoError(t, err)
		require.NotNil(t, sessionFiles)

		// Manager should get the host file
		require.True(t, len(sessionFiles.HostFile) > 0, "manager should receive the host file")
		// Manager should get all turn files
		require.Equal(t, 2, len(sessionFiles.Turns), "manager should receive all turn files")
		// Manager should get the universe file
		require.True(t, len(sessionFiles.Universe) > 0, "manager should receive the universe file")
	})

	t.Run("global_manager_can_get_all_files", func(t *testing.T) {
		// A global manager who is not a member of the session
		globalManagerPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "someOtherUserID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}
		params := operations.SessionFilesGetParams{
			SessionID: "merryvsgollumID",
		}
		sessionFiles, err := handler.handle(ctx, params, globalManagerPrincipal)
		require.NoError(t, err)
		require.NotNil(t, sessionFiles)

		// Global manager should get the host file
		require.True(t, len(sessionFiles.HostFile) > 0, "global manager should receive the host file")
	})

	t.Run("regular_member_cannot_get_files", func(t *testing.T) {
		// Gollum is a regular member, not a manager
		gollumPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "gollumID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.SessionFilesGetParams{
			SessionID: "merryvsgollumID",
		}
		_, err := handler.handle(ctx, params, gollumPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden), "regular member should get forbidden error")
	})

	t.Run("non_member_cannot_get_files", func(t *testing.T) {
		// Someone who is not a member at all
		nonMemberPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "randomUserID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.SessionFilesGetParams{
			SessionID: "merryvsgollumID",
		}
		_, err := handler.handle(ctx, params, nonMemberPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden), "non-member should get forbidden error")
	})

	t.Run("not_found_when_no_session_files", func(t *testing.T) {
		// Load a session without turn files
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")

		merryPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true, // Use global manager to bypass membership check
		}
		params := operations.SessionFilesGetParams{
			SessionID: "gondorID", // Session without any files
		}
		_, err := handler.handle(ctx, params, merryPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrNotFound), "should get not found when no session files exist")
	})
}
