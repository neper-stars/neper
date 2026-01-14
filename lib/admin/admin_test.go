package admin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
)

func TestEnsureAdminUser(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	t.Run("creates_admin_user_when_not_exists", func(t *testing.T) {
		opts := &Options{
			AutocreateAdmin: true,
			AdminUsername:   "testadmin",
			AdminEmail:      "testadmin@example.com",
		}

		apiKey, err := EnsureAdminUser(ctx, testdb.DB, &log, opts)
		require.NoError(t, err)
		require.NotEmpty(t, apiKey, "should return generated API key")

		// Verify user exists in database
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var user models.UserProfileDB
		err = sqlH.GetWhere(&user, map[string]interface{}{
			models.UserProfileDBNicknameColumn: "testadmin",
		})
		require.NoError(t, err)
		require.Equal(t, "testadmin", user.Nickname)
		require.Equal(t, "testadmin@example.com", user.Email)
		require.True(t, user.IsManager, "admin user should be a manager")
		require.Equal(t, models.UserProfileStateActive, user.State, "admin user should be active")
		require.Equal(t, apiKey, user.APIKey)
	})

	t.Run("idempotent_does_not_create_duplicate", func(t *testing.T) {
		opts := &Options{
			AutocreateAdmin: true,
			AdminUsername:   "testadmin",
			AdminEmail:      "testadmin@example.com",
		}

		// Second call should not create a new user
		apiKey, err := EnsureAdminUser(ctx, testdb.DB, &log, opts)
		require.NoError(t, err)
		require.Empty(t, apiKey, "should return empty API key for existing user")

		// Verify still only one user with this nickname
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var count int
		err = sqlH.Get(&count, database.SQ.Select("COUNT(*)").
			From(models.UserProfileDBTable).
			Where(map[string]interface{}{
				models.UserProfileDBNicknameColumn: "testadmin",
			}))
		require.NoError(t, err)
		require.Equal(t, 1, count)
	})

	t.Run("uses_provided_apikey", func(t *testing.T) {
		opts := &Options{
			AutocreateAdmin: true,
			AdminUsername:   "testadmin2",
			AdminEmail:      "testadmin2@example.com",
			AdminAPIKey:     "myspecialapikey123",
		}

		apiKey, err := EnsureAdminUser(ctx, testdb.DB, &log, opts)
		require.NoError(t, err)
		require.Equal(t, "myspecialapikey123", apiKey)

		// Verify user has the provided API key
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var user models.UserProfileDB
		err = sqlH.GetWhere(&user, map[string]interface{}{
			models.UserProfileDBNicknameColumn: "testadmin2",
		})
		require.NoError(t, err)
		require.Equal(t, "myspecialapikey123", user.APIKey)
	})

	t.Run("generates_email_if_not_provided", func(t *testing.T) {
		opts := &Options{
			AutocreateAdmin: true,
			AdminUsername:   "testadmin3",
			// No email provided
		}

		_, err := EnsureAdminUser(ctx, testdb.DB, &log, opts)
		require.NoError(t, err)

		// Verify user has generated email
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var user models.UserProfileDB
		err = sqlH.GetWhere(&user, map[string]interface{}{
			models.UserProfileDBNicknameColumn: "testadmin3",
		})
		require.NoError(t, err)
		require.Equal(t, "testadmin3@neper.local", user.Email)
	})

	t.Run("disabled_when_autocreate_false", func(t *testing.T) {
		opts := &Options{
			AutocreateAdmin: false,
			AdminUsername:   "testadmin4",
			AdminEmail:      "test@example.com",
		}

		require.False(t, opts.Enabled())

		apiKey, err := EnsureAdminUser(ctx, testdb.DB, &log, opts)
		require.NoError(t, err)
		require.Empty(t, apiKey)
	})
}
