package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/fixtures"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
	"github.com/neper-stars/neper/sync"
)

func TestSessionUpdateHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// create sessions
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	// create user without session
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")

	handler := NewSessionUpdateHandler(&log, testdb.DB)
	readHandler := NewSessionReadHandler(&log, testdb.DB)

	t.Run("update shire", func(t *testing.T) {
		shireID := "shireID"
		p := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		readParams := operations.SessionReadParams{
			SessionID: "shireID",
		}
		initialShire, err := readHandler.handle(ctx, readParams, p)
		require.NoError(t, err)
		// the shire has no members initially
		require.Equal(t, 0, len(initialShire.Members))
		require.Equal(t, "The Shire", initialShire.Name)

		initialShire.Members = []string{"merryID"} // merry is one of our users !
		initialShire.Name = "The Shire Updated"

		// update the Shire as general manager
		gandalfPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "gandalfID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}
		updateParams := operations.SessionUpdateParams{
			Session:   initialShire,
			SessionID: initialShire.ID,
		}
		session, err := handler.handle(ctx, updateParams, gandalfPrincipal)

		require.NoError(t, err)
		require.Equal(t, "The Shire Updated", session.Name)
		require.Equal(t, shireID, session.ID)

		// reread from our api to see if members are correctly set
		shireSession, err := readHandler.handle(ctx, readParams, gandalfPrincipal)
		require.NoError(t, err)
		// name was updated
		require.Equal(t, session.Name, shireSession.Name)
		// 1 member was added
		require.Equal(t, 1, len(shireSession.Members))
		require.Equal(t, "merryID", shireSession.Members[0])
	})
}
