package fixtureloader

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/ulikunitz/xz"

	"github.com/neper-stars/neper/fixtures"
	"github.com/neper-stars/neper/sync"
)

type Loader struct {
	db     *sqlx.DB
	log    zerolog.Logger
	worker *sync.Worker
}

func NewLoader(tb testing.TB, db *sqlx.DB, log zerolog.Logger) *Loader {
	tb.Helper()
	w, err := sync.NewWorker(db, log)
	require.NoError(tb, err)
	return &Loader{
		db:     db,
		log:    log,
		worker: w,
	}
}

func newCompressedReader(reader io.Reader) (*compressedReader, error) {
	r := compressedReader{
		closeFn: func() error { return nil },
	}

	if c, ok := reader.(io.Closer); ok {
		r.closeFn = c.Close
	}

	uncomp, err := xz.NewReader(reader)
	if err != nil {
		_ = r.Close()
		return nil, err
	}

	r.Reader = uncomp

	return &r, nil
}

type compressedReader struct {
	io.Reader
	closeFn func() error
}

func (r compressedReader) Close() error {
	if r.closeFn != nil {
		return r.closeFn()
	}
	return nil
}

func openFile(tb testing.TB, filename string) io.ReadCloser {
	tb.Helper()
	var reader io.ReadCloser
	f, err := os.Open(filename)
	require.NoError(tb, err)
	reader = f
	if strings.HasSuffix(filename, ".xz") {
		reader, err = newCompressedReader(reader)
		require.NoError(tb, err)
	}

	return reader
}

func (l *Loader) Load(tb testing.TB, data io.Reader) {
	tb.Helper()
	fixtures.LoadFixture(tb, l.worker, data)
}

func (l *Loader) LoadJSON(tb testing.TB, data []byte) {
	tb.Helper()
	fixtures.LoadFixtureJSON(tb, l.worker, data)
}

func (l *Loader) LoadFile(tb testing.TB, filename string) {
	tb.Helper()
	rc := openFile(tb, filename)
	defer rc.Close()
	l.Load(tb, rc)
}

func Load(tb testing.TB, db *sqlx.DB, data io.Reader, log zerolog.Logger) {
	tb.Helper()
	NewLoader(tb, db, log).Load(tb, data)
}

func LoadJSON(tb testing.TB, db *sqlx.DB, data []byte, log zerolog.Logger) {
	tb.Helper()
	NewLoader(tb, db, log).LoadJSON(tb, data)
}

func LoadFile(tb testing.TB, db *sqlx.DB, filename string, log zerolog.Logger) {
	tb.Helper()
	NewLoader(tb, db, log).LoadFile(tb, filename)
}
