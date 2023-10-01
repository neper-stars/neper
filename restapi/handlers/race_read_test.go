package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/fixtures"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/sync"
)

func TestRaceReadHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// load some sessions because our players are members of them
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	// humans are owned by Boromir
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	// hobbits are owned by Merry
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")
	// load races after players, hobbits for Merry and Humans for Boromir
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/races.json")

	handler := NewRaceReadHandler(testdb.DB)

	t.Run("humans", func(t *testing.T) {
		// here we do not test authorizations which are applied in the
		// layer just above the handler we are testing.
		race, err := handler.handle(ctx, "humansID")

		require.NoError(t, err)
		require.Equal(t, "Humans", race.NamePlural)
	})
}
