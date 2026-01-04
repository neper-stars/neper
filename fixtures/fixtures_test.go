package fixtures

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/sync"
)

func TestLoadFixture(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	w, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	LoadFixtureJSON(t, w, []byte(`{
		"changes": [{
			"operation": "create",
			"data": {
				"__type__": "session",
				"id": "1",
				"name": "session 1",
				"state": "pending"
			}
		}]
	}`))
}

func TestLoadFixtureFile(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	w, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	LoadFixtureFile(t, w, "fixturefile.json")
}
