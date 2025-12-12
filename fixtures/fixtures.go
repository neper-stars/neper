package fixtures

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/testutils"
)

// FixtureWorker ...
type FixtureWorker interface {
	DispatchMessage(ctx context.Context, msgType string, data io.Reader, log zerolog.Logger) error
}

// LoadFixture loads a fixture set into the database
func LoadFixture(tb testing.TB, worker FixtureWorker, data io.Reader) {
	tb.Helper()
	log := testutils.GetLogger(tb)

	require.NoError(tb, worker.DispatchMessage(
		context.Background(),
		"neper-referential-broadcast",
		data,
		log,
	))
}

// LoadFixtureJSON loads a json-encoded fixture
func LoadFixtureJSON(tb testing.TB, worker FixtureWorker, data []byte) {
	tb.Helper()
	LoadFixture(tb, worker, bytes.NewBuffer(data))
}

// LoadFixtureFile loads a json-encoded fixture file
func LoadFixtureFile(tb testing.TB, consumer FixtureWorker, filename string) {
	tb.Helper()
	b, err := os.ReadFile(filename) // #nosec G304 -- test fixtures from trusted source
	if err != nil {
		tb.Fatal(err)
	}
	LoadFixtureJSON(tb, consumer, b)
}
