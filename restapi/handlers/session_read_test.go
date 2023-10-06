package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/fixtures"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
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

	handler := NewSessionReadHandler(&log, testdb.DB)

	t.Run("empty Gondor", func(t *testing.T) {
		p := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.SessionReadParams{
			SessionID: "gondorID",
		}
		session, err := handler.handle(ctx, params, p)

		require.NoError(t, err)
		require.Equal(t, "Gondor", session.Name)
		require.Equal(t, 0, len(session.Members))
		require.Equal(t, 0, len(session.Managers))
	})

	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gandalf.json")

	t.Run("Gondor with Gandalf as manager", func(t *testing.T) {
		p := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.SessionReadParams{
			SessionID: "gondorID",
		}
		session, err := handler.handle(ctx, params, p)
		require.NoError(t, err)
		require.Equal(t, "Gondor", session.Name)
		require.Equal(t, 0, len(session.Members))
		require.Equal(t, 1, len(session.Managers))
		require.Equal(t, "gandalfID", session.Managers[0])
	})

	t.Run("isengard_is_private_and_merry_is_not_a_member", func(t *testing.T) {
		p := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		params := operations.SessionReadParams{
			SessionID: "isengardID",
		}
		_, err := handler.handle(ctx, params, p)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden))
	})
}
