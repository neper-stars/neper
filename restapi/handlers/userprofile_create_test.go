package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
)

func TestUserProfileCreateHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	handler := NewUserProfileCreateHandler(testdb.DB)
	readHandler := NewUserProfileReadHandler(testdb.DB)

	t.Run("create gandalf", func(t *testing.T) {
		// here we do not test authorizations which are applied in the
		// layer just above the handler we are testing.
		// We can only test if the returned content is coherent with
		// our current user
		gandalf := &models.UserProfile{
			ID:        "gandalfID",
			Nickname:  "G",
			Email:     "gandalf@shire.com",
			IsActive:  true,
			IsManager: true,
		}
		user, err := handler.handle(ctx, gandalf)

		require.NoError(t, err)
		require.Equal(t, gandalf.Nickname, user.Nickname)
		// ID is a readOnly field and will be filled with something by the API
		require.NotEqual(t, "", user.ID)

		// reread from our api to see if members are correctly set
		gandalfUser, err := readHandler.handle(ctx, user.ID)
		require.NoError(t, err)
		require.Equal(t, gandalf.Nickname, gandalfUser.Nickname)
		assert.Equal(t, gandalf.IsManager, gandalfUser.IsManager)
	})
}
