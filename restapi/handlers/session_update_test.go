package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/fixtures"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/sync"
)

func TestSessionUpdateHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	authZ := auth.NewAuthorizer(log, testdb.DB)
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// create sessions
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	// create user without session
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")

	handler := NewSessionUpdateHandler(testdb.DB, authZ)
	readHandler := NewSessionReadHandler(testdb.DB)

	t.Run("update shire", func(t *testing.T) {
		shireID := "shireID"
		initialShire, err := readHandler.handle(ctx, shireID)
		require.NoError(t, err)
		// the shire has no members initially
		require.Equal(t, 0, len(initialShire.Members))
		require.Equal(t, "The Shire", initialShire.Name)

		initialShire.Members = []string{"merryID"} // merry is one of our users !
		initialShire.Name = "The Shire Updated"

		// update the Shire
		session, err := handler.handle(ctx, initialShire)

		require.NoError(t, err)
		require.Equal(t, "The Shire Updated", session.Name)
		require.Equal(t, shireID, session.ID)

		// reread from our api to see if members are correctly set
		shireSession, err := readHandler.handle(ctx, session.ID)
		require.NoError(t, err)
		// name was updated
		require.Equal(t, session.Name, shireSession.Name)
		// 1 member was added
		require.Equal(t, 1, len(shireSession.Members))
		require.Equal(t, "merryID", shireSession.Members[0])
	})
}
