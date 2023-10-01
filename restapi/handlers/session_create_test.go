package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/auth"
	"github.com/neper-stars/neper/fixtures"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/sync"
)

func TestSessionCreateHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	authZ := auth.NewAuthorizer(log, testdb.DB)
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")

	handler := NewSessionCreateHandler(testdb.DB, authZ)
	readHandler := NewSessionReadHandler(testdb.DB)

	t.Run("create shire", func(t *testing.T) {
		// here we do not test authorizations which are applied in the
		// layer just above the handler we are testing.
		// We can only test if the returned content is coherent with
		// our current user
		shire := &models.Session{
			Managers: []string{"merryID"}, // merry is one of our users !!
			Name:     "The Shire",
		}
		p := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		session, err := handler.handle(ctx, shire, p)

		require.NoError(t, err)
		require.Equal(t, "The Shire", session.Name)
		// ID is a readOnly field and will be filled with something by the API
		require.NotEqual(t, "", session.ID)

		// reread from our api to see if members are correctly set
		shireSession, err := readHandler.handle(ctx, session.ID)
		require.NoError(t, err)
		require.Equal(t, session.Name, shireSession.Name)
		require.Equal(t, 1, len(shireSession.Managers))
		require.Equal(t, "merryID", shireSession.Managers[0])
	})
}
