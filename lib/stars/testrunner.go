package stars

import (
	"os"
	"testing"

	"github.com/m4rw3r/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type ShutdownFunc func()

// GetTestStarsRunner is the easiest way to get a runner for testing.
// This returns a test runner
// Note: Xvfb shutdown and cleanup of directories is registered
// via t.Cleanup() to ensure cleanup happens even if the test fails or panics.
func GetTestStarsRunner(t *testing.T, log *zerolog.Logger) *Runner {
	t.Helper()
	return GetTestStarsRunnerWithAutoDelete(t, log, true)
}

// GetTestStarsRunnerWithAutoDelete is like GetTestStarsRunner but allows
// controlling whether directories are automatically deleted after the test.
// When autoDelete is false, the wine prefix, executable dir, and save dir
// will be preserved after the test completes (useful for debugging).
func GetTestStarsRunnerWithAutoDelete(t *testing.T, log *zerolog.Logger, autoDelete bool) *Runner {
	t.Helper()
	// each runner gets its own UID
	uid, err := uuid.V4()
	require.NoError(t, err)
	// We use a home root dir because this is something we do not want in our source tree
	// and this is also something that could be kept from one run to another
	// Not sure if we should do this. We could put this in the tests dir and .hgignore it
	testWinePrefix := "~/.neper/wine" + uid.String()
	testExecutableDir := "~/.neper/stars" + uid.String()
	testSaveDir := "~/.neper/saves" + uid.String()
	opts := RunnerOptions{
		ExecutableDir:   testExecutableDir,
		SaveDir:         testSaveDir,
		WinePrefix:      testWinePrefix,
		CommandsTimeout: 60,
		DisplayNumber:   98,
	}
	runner, err := NewRunner(log, &opts)
	if err != nil {
		require.NoError(t, err, "failed to create runner")
	}
	if err := runner.Init(); err != nil {
		require.NoError(t, err, "failed to initialize runner")
	}

	// Register cleanup via t.Cleanup to ensure directories are removed
	// even if the test fails or panics (only if autoDelete is true)
	if autoDelete {
		t.Cleanup(func() {
			_ = os.RemoveAll(runner.Prefix().Path())
			_ = os.RemoveAll(runner.ExecutableDir)
			_ = os.RemoveAll(runner.SaveDir)
			runner.Shutdown()
		})
	}

	return runner
}
