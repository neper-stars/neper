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

func TestSessionReadHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// we only load sessions as gandalf.json contains users/perms that are not
	// tested here
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")

	handler := NewSessionReadHandler(testdb.DB)

	t.Run("empty Gondor", func(t *testing.T) {
		// here we do not test authorizations which are applied in the
		// layer just above the handler we are testing.
		// We can only test if the returned content is coherent with
		// our current user
		session, err := handler.handle(ctx, "gondorID")

		require.NoError(t, err)
		require.Equal(t, "Gondor", session.Name)
		require.Equal(t, 0, len(session.Members))
		require.Equal(t, 0, len(session.Managers))
	})

	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gandalf.json")

	t.Run("Gondor with Gandalf as manager", func(t *testing.T) {
		session, err := handler.handle(ctx, "gondorID")
		require.NoError(t, err)
		require.Equal(t, "Gondor", session.Name)
		require.Equal(t, 0, len(session.Members))
		require.Equal(t, 1, len(session.Managers))
		require.Equal(t, "gandalfID", session.Managers[0])
	})
}
