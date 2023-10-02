package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func TestUserProfileCreateHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gandalf.json")

	handler := NewUserProfileCreateHandler(testdb.DB)
	readHandler := NewUserProfileReadHandler(testdb.DB)

	t.Run("create gandalf", func(t *testing.T) {
		gandalfPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "gandalfID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}
		saroumane := &models.UserProfile{
			ID:        "saroumaneID",
			Nickname:  "Saroumane",
			Email:     "saroumane@isengard.com",
			IsActive:  true,
			IsManager: false,
		}
		createParams := operations.UserProfileCreateParams{
			UserProfile: saroumane,
		}
		user, err := handler.handle(ctx, createParams, &gandalfPrincipal)

		require.NoError(t, err)
		require.Equal(t, saroumane.Nickname, user.Nickname)
		// ID is a readOnly field and will be filled with something by the API
		require.NotEqual(t, "", user.ID)

		// reread from our api to see if members are correctly set
		saroumaneUser, err := readHandler.handle(ctx, user.ID)
		require.NoError(t, err)
		require.Equal(t, saroumane.Nickname, saroumaneUser.Nickname)
		assert.Equal(t, saroumane.IsManager, saroumaneUser.IsManager)
	})
}
