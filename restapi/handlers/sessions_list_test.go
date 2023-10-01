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
	"github.com/neper-stars/neper/sync"
)

func TestSessionsListHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// we only load sessions as gandalf.json contains users/perms that are not
	// tested here
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gandalf.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")

	handler := NewSessionsListHandler(testdb.DB)

	// here we do not test authorizations which are applied in the
	// layer just above the handler we are testing.
	// We can only test if the returned content is coherent with
	// our current user
	t.Run("gandalf_is_general_manager_and_sees_all", func(t *testing.T) {
		sessions, err := handler.handle(ctx, &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "gandalfID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		})
		require.NoError(t, err)
		require.Equal(t, 3, len(sessions))
	})

	t.Run("boromir_is_gondor_member_and_sees_gondor_only", func(t *testing.T) {
		sessions, err := handler.handle(ctx, &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		})
		require.NoError(t, err)
		require.Equal(t, 1, len(sessions))
		require.Equal(t, "gondorID", sessions[0].ID)
	})
}
