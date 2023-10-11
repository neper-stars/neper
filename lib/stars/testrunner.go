package stars

import (
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type ShutdownFunc func()

// GetTestStarsRunner is the easiest way to get a runner for testing
// this returns a test runner and a ShutdownFunc that must be called after your test
// is done if you want the test Xvfb server to shut down
func GetTestStarsRunner(t *testing.T, log *zerolog.Logger) (*Runner, ShutdownFunc) {
	t.Helper()
	// We use a home root dir because this is something we do not want in our source tree
	// and this is also something that could be kept from one run to another
	// Not sure if we should do this. We could put this in the tests dir and .hgignore it
	testWinePrefix := "~/.neper/wine"
	testExecutableDir := "~/.neper/stars"
	testSaveDir := "~/.neper/saves"
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
	return runner, func() {
		require.NoError(t, os.RemoveAll(runner.WinePrefix))
		require.NoError(t, os.RemoveAll(runner.ExecutableDir))
		require.NoError(t, os.RemoveAll(runner.SaveDir))
		runner.Shutdown()
	}
}
