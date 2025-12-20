package handlers

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
)

func TestUserProfileDeleteHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	createHandler := NewUserProfileCreateHandler(testdb.DB)
	deleteHandler := NewUserProfileDeleteHandler(&log, testdb.DB)

	adminPrincipal := &models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "adminID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		IsGlobalManager: true,
	}

	regularPrincipal := &models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "regularID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		IsGlobalManager: false,
	}

	t.Run("global_manager_can_delete_user", func(t *testing.T) {
		// Create a user first
		createParams := operations.UserProfileCreateParams{
			UserProfile: &models.UserProfileCreateRequest{
				Nickname: "todelete",
				Email:    "todelete@example.com",
			},
		}
		user, err := createHandler.handle(ctx, createParams, adminPrincipal)
		require.NoError(t, err)

		// Delete the user
		deleteParams := operations.UserProfileDeleteParams{
			UserProfileID: user.ID,
		}
		err = deleteHandler.handle(ctx, deleteParams, adminPrincipal)
		require.NoError(t, err)

		// Verify the user is deleted
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var userDB models.UserProfileDB
		err = sqlH.GetByPKey(&userDB, user.ID)
		require.Error(t, err)
		require.True(t, errors.Is(err, sql.ErrNoRows), "user should be deleted")
	})

	t.Run("regular_user_cannot_delete", func(t *testing.T) {
		// Create a user first
		createParams := operations.UserProfileCreateParams{
			UserProfile: &models.UserProfileCreateRequest{
				Nickname: "nodelete",
				Email:    "nodelete@example.com",
			},
		}
		user, err := createHandler.handle(ctx, createParams, adminPrincipal)
		require.NoError(t, err)

		// Try to delete as regular user
		deleteParams := operations.UserProfileDeleteParams{
			UserProfileID: user.ID,
		}
		err = deleteHandler.handle(ctx, deleteParams, regularPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden))
	})

	t.Run("delete_nonexistent_user_returns_not_found", func(t *testing.T) {
		deleteParams := operations.UserProfileDeleteParams{
			UserProfileID: "nonexistent-id",
		}
		err := deleteHandler.handle(ctx, deleteParams, adminPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrNotFound))
	})

	t.Run("cannot_delete_user_in_session", func(t *testing.T) {
		// Create a user
		createParams := operations.UserProfileCreateParams{
			UserProfile: &models.UserProfileCreateRequest{
				Nickname: "insession",
				Email:    "insession@example.com",
			},
		}
		user, err := createHandler.handle(ctx, createParams, adminPrincipal)
		require.NoError(t, err)

		// Create a session and add the user to it
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		_, err = sqlH.Exec(database.SQ.
			Insert("session").
			Columns("id", "name", "private", "rules_is_set", "started").
			Values("test-session-id", "Test Session", false, false, false))
		require.NoError(t, err)

		_, err = sqlH.Exec(database.SQ.
			Insert("user_profile_session_rel").
			Columns("user_profile_id", "session_id", "is_manager").
			Values(user.ID, "test-session-id", false))
		require.NoError(t, err)

		// Try to delete the user
		deleteParams := operations.UserProfileDeleteParams{
			UserProfileID: user.ID,
		}
		err = deleteHandler.handle(ctx, deleteParams, adminPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrConflict), "should get conflict error for user in session")
	})
}
