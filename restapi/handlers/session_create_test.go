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

func TestSessionCreateHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")

	createHandler := NewSessionCreateHandler(testdb.DB, nil)
	readHandler := NewSessionReadHandler(&log, testdb.DB)

	t.Run("create shire", func(t *testing.T) {
		// here we do not test authorizations which are applied in the
		// layer just above the handler we are testing.
		// We can only test if the returned content is coherent with
		// our current user
		shire := &models.Session{
			Managers: []string{"merryID"}, // merry is one of our users !!
			Name:     "The Shire",
		}
		merryPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.SessionCreateParams{
			Session: shire,
		}
		session, err := createHandler.handle(ctx, params, merryPrincipal)

		require.NoError(t, err)
		require.Equal(t, "The Shire", session.Name)
		// ID is a readOnly field and will be filled with something by the API
		require.NotEqual(t, "", session.ID)
		require.Equal(t, []string{"merryID"}, session.Managers)

		// reread from our api to see if members are correctly set
		readParams := operations.SessionReadParams{
			SessionID: session.ID,
		}
		shireSession, err := readHandler.handle(ctx, readParams, merryPrincipal)
		require.NoError(t, err)
		require.Equal(t, session.Name, shireSession.Name)
		require.Equal(t, 1, len(shireSession.Managers))
		require.Equal(t, "merryID", shireSession.Managers[0])
		require.Equal(t, false, shireSession.Private)
	})

	t.Run("create mordor", func(t *testing.T) {
		// here we do not test authorizations which are applied in the
		// layer just above the handler we are testing.
		// We can only test if the returned content is coherent with
		// our current user
		mordor := &models.Session{
			Managers: []string{"merryID"}, // merry is one of our users !!
			Name:     "Mordor",
			Private:  true,
		}
		merryPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.SessionCreateParams{
			Session: mordor,
		}
		session, err := createHandler.handle(ctx, params, merryPrincipal)

		require.NoError(t, err)
		require.Equal(t, "Mordor", session.Name)
		// ID is a readOnly field and will be filled with something by the API
		require.NotEqual(t, "", session.ID)
		require.Equal(t, []string{"merryID"}, session.Managers)

		// reread from our api to see if members are correctly set
		readParams := operations.SessionReadParams{
			SessionID: session.ID,
		}
		mordorSession, err := readHandler.handle(ctx, readParams, merryPrincipal)
		require.NoError(t, err)
		require.Equal(t, session.Name, mordorSession.Name)
		require.Equal(t, 1, len(mordorSession.Managers))
		require.Equal(t, "merryID", mordorSession.Managers[0])
		// mordor was made private on purpose! so it should be returned as private
		require.Equal(t, true, mordorSession.Private)
	})
}
