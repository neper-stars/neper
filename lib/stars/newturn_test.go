package stars

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/testutils"
)

func TestCleanupSessionDir(t *testing.T) {
	log := testutils.GetLogger(t)
	runner := GetTestStarsRunner(t, &log)

	sessionID := "test-cleanup-session"

	t.Run("cleanup_removes_session_directory", func(t *testing.T) {
		// Create a session directory with some files
		sessionDir := runner.SessionDir(sessionID)
		err := os.MkdirAll(sessionDir, 0770)
		require.NoError(t, err)

		// Create a dummy file in the directory
		dummyFile := sessionDir + "/game.xy"
		err = os.WriteFile(dummyFile, []byte("test data"), 0660) //nolint:gosec // Stars! executable requires group write access
		require.NoError(t, err)

		// Verify directory exists
		_, err = os.Stat(sessionDir)
		require.NoError(t, err, "session directory should exist before cleanup")

		// Cleanup
		err = runner.CleanupSessionDir(sessionID)
		require.NoError(t, err)

		// Verify directory is gone
		_, err = os.Stat(sessionDir)
		require.True(t, os.IsNotExist(err), "session directory should not exist after cleanup")
	})

	t.Run("cleanup_nonexistent_directory_succeeds", func(t *testing.T) {
		// Cleanup a directory that doesn't exist should not error
		err := runner.CleanupSessionDir("nonexistent-session-id")
		require.NoError(t, err, "cleanup of nonexistent directory should succeed")
	})
}
