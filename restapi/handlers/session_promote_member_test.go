package handlers

import (
	"context"
	"testing"
	"time"

	sq "github.com/Masterminds/squirrel"
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

func TestSessionPromoteMemberHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")

	promoteHandler := NewSessionPromoteMemberHandler(&log, testdb.DB, nil)

	t.Run("manager_can_promote_member", func(t *testing.T) {
		sessionID := "gondorID"
		targetUserID := "finduilasID"

		// boromir is manager of gondor
		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Verify finduilas is not a manager before promotion
		tx, err := database.Begin(ctx, testdb.DB)
		require.NoError(t, err)
		sqlH := database.NewSQLHelper(ctx, tx, log)
		isManager, err := IsSessionManager(sqlH, targetUserID, sessionID)
		require.NoError(t, err)
		require.False(t, isManager, "finduilas should not be a manager before promotion")
		tx.RollbackIfOpened(log)

		// Promote finduilas
		promoteParams := operations.SessionPromoteMemberParams{
			SessionID:     sessionID,
			UserProfileID: targetUserID,
		}
		err = promoteHandler.handle(ctx, promoteParams, &boromirPrincipal)
		require.NoError(t, err)

		// Verify finduilas is now a manager
		tx, err = database.Begin(ctx, testdb.DB)
		require.NoError(t, err)
		sqlH = database.NewSQLHelper(ctx, tx, log)
		isManager, err = IsSessionManager(sqlH, targetUserID, sessionID)
		require.NoError(t, err)
		require.True(t, isManager, "finduilas should be a manager after promotion")
		tx.RollbackIfOpened(log)
	})

	t.Run("global_manager_can_promote_member", func(t *testing.T) {
		sessionID := "gondorID"

		// First demote finduilas back to regular member
		tx, err := database.Begin(ctx, testdb.DB)
		require.NoError(t, err)
		sqlH := database.NewSQLHelper(ctx, tx, log)
		_, err = sqlH.Exec(database.SQ.Update(models.UserProfileSessionRelDBTable).
			Set(models.UserProfileSessionRelDBIsManagerColumn, false).
			Where(sq.And{
				sq.Eq{models.UserProfileSessionRelDBUserProfileIDColumn: "finduilasID"},
				sq.Eq{models.UserProfileSessionRelDBSessionIDColumn: sessionID},
			}))
		require.NoError(t, err)
		require.NoError(t, tx.Commit())

		// A global manager who is not a member of the session
		globalManagerPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "someGlobalManagerID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		// Promote finduilas as global manager
		promoteParams := operations.SessionPromoteMemberParams{
			SessionID:     sessionID,
			UserProfileID: "finduilasID",
		}
		err = promoteHandler.handle(ctx, promoteParams, &globalManagerPrincipal)
		require.NoError(t, err)

		// Verify finduilas is now a manager
		tx, err = database.Begin(ctx, testdb.DB)
		require.NoError(t, err)
		sqlH = database.NewSQLHelper(ctx, tx, log)
		isManager, err := IsSessionManager(sqlH, "finduilasID", sessionID)
		require.NoError(t, err)
		require.True(t, isManager, "finduilas should be a manager after promotion by global manager")
		tx.RollbackIfOpened(log)
	})

	t.Run("regular_member_cannot_promote", func(t *testing.T) {
		sessionID := "gondorID"

		// First demote finduilas back to regular member
		tx, err := database.Begin(ctx, testdb.DB)
		require.NoError(t, err)
		sqlH := database.NewSQLHelper(ctx, tx, log)
		_, err = sqlH.Exec(database.SQ.Update(models.UserProfileSessionRelDBTable).
			Set(models.UserProfileSessionRelDBIsManagerColumn, false).
			Where(sq.And{
				sq.Eq{models.UserProfileSessionRelDBUserProfileIDColumn: "finduilasID"},
				sq.Eq{models.UserProfileSessionRelDBSessionIDColumn: sessionID},
			}))
		require.NoError(t, err)
		require.NoError(t, tx.Commit())

		// finduilas (regular member) tries to promote someone
		finduilasPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "finduilasID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		promoteParams := operations.SessionPromoteMemberParams{
			SessionID:     sessionID,
			UserProfileID: "boromirID", // trying to "promote" the existing manager
		}
		err = promoteHandler.handle(ctx, promoteParams, &finduilasPrincipal)
		require.Error(t, err)
		require.ErrorIs(t, err, errs.ErrForbidden)
	})

	t.Run("cannot_promote_non_member", func(t *testing.T) {
		sessionID := "gondorID"

		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Try to promote someone who is not a member
		promoteParams := operations.SessionPromoteMemberParams{
			SessionID:     sessionID,
			UserProfileID: "nonexistentUserID",
		}
		err := promoteHandler.handle(ctx, promoteParams, &boromirPrincipal)
		require.Error(t, err)
		require.ErrorIs(t, err, errs.ErrPreconditionFailed)
		require.Contains(t, err.Error(), "not a member")
	})

	t.Run("promote_in_nonexistent_session", func(t *testing.T) {
		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		promoteParams := operations.SessionPromoteMemberParams{
			SessionID:     "nonexistent-session-id",
			UserProfileID: "finduilasID",
		}
		err := promoteHandler.handle(ctx, promoteParams, &boromirPrincipal)
		require.Error(t, err)
		require.Contains(t, err.Error(), "session not found")
	})

	t.Run("promoting_existing_manager_is_idempotent", func(t *testing.T) {
		sessionID := "gondorID"

		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Try to promote boromir (who is already a manager)
		promoteParams := operations.SessionPromoteMemberParams{
			SessionID:     sessionID,
			UserProfileID: "boromirID",
		}
		err := promoteHandler.handle(ctx, promoteParams, &boromirPrincipal)
		require.NoError(t, err, "promoting an existing manager should succeed (idempotent)")
	})
}
