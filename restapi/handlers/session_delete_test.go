package handlers

import (
	"context"
	"database/sql"
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

func TestSessionDeleteHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	// Load fixtures
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")

	deleteHandler := NewSessionDeleteHandler(&log, testdb.DB, nil)

	t.Run("session_manager_can_delete_session", func(t *testing.T) {
		// Boromir is manager of gondor session (session_list has is_manager: true)
		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		deleteParams := operations.SessionDeleteParams{
			SessionID: "gondorID",
		}

		err := deleteHandler.handle(ctx, deleteParams, &boromirPrincipal)
		require.NoError(t, err)

		// Verify session is deleted
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var count int
		countQuery := sq.Select("COUNT(*)").
			From(models.SessionDBTable).
			Where(sq.Eq{models.SessionDBIDColumn: "gondorID"})
		err = sqlH.Get(&count, countQuery)
		require.NoError(t, err)
		require.Equal(t, 0, count, "session should be deleted")
	})
}

func TestSessionDeleteHandler_Authorization(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	// Load fixtures
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")

	deleteHandler := NewSessionDeleteHandler(&log, testdb.DB, nil)

	t.Run("member_cannot_delete_session", func(t *testing.T) {
		// Finduilas is member but not manager
		finduilasPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "finduilasID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		deleteParams := operations.SessionDeleteParams{
			SessionID: "gondorID",
		}

		err := deleteHandler.handle(ctx, deleteParams, &finduilasPrincipal)
		require.Error(t, err)
		require.ErrorIs(t, err, errs.ErrForbidden)
	})

	t.Run("global_manager_can_delete_session", func(t *testing.T) {
		globalManagerPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "someRandomUserID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		// Use shireID instead of gondorID since we're testing authorization on a different session
		deleteParams := operations.SessionDeleteParams{
			SessionID: "shireID",
		}

		err := deleteHandler.handle(ctx, deleteParams, &globalManagerPrincipal)
		require.NoError(t, err)
	})
}

func TestSessionDeleteHandler_NotFound(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	deleteHandler := NewSessionDeleteHandler(&log, testdb.DB, nil)

	t.Run("delete_nonexistent_session_returns_not_found", func(t *testing.T) {
		principal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "someUserID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		deleteParams := operations.SessionDeleteParams{
			SessionID: "nonexistent-session-id",
		}

		err := deleteHandler.handle(ctx, deleteParams, &principal)
		require.Error(t, err)
		require.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestSessionDeleteHandler_CascadeDelete(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	// Load fixtures with related data
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_vs_gollum_ruleset.json")

	deleteHandler := NewSessionDeleteHandler(&log, testdb.DB, nil)

	t.Run("deleting_session_cascades_to_related_tables", func(t *testing.T) {
		sessionID := "merryvsgollumID"

		// Verify data exists before delete
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)

		var sprCount int
		sprQuery := sq.Select("COUNT(*)").
			From(models.SessionPlayerRaceDBTable).
			Where(sq.Eq{models.SessionPlayerRaceDBSessionIDColumn: sessionID})
		err := sqlH.Get(&sprCount, sprQuery)
		require.NoError(t, err)
		require.Greater(t, sprCount, 0, "should have session player races before delete")

		var rulesetCount int
		rulesetQuery := sq.Select("COUNT(*)").
			From(models.RulesetDBTable).
			Where(sq.Eq{models.RulesetDBSessionIDColumn: sessionID})
		err = sqlH.Get(&rulesetCount, rulesetQuery)
		require.NoError(t, err)
		require.Greater(t, rulesetCount, 0, "should have ruleset before delete")

		// Delete session as global manager
		principal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		deleteParams := operations.SessionDeleteParams{
			SessionID: sessionID,
		}

		err = deleteHandler.handle(ctx, deleteParams, &principal)
		require.NoError(t, err)

		// Verify session is deleted
		var sessionDB models.SessionDB
		err = sqlH.GetByPKey(&sessionDB, sessionID)
		require.ErrorIs(t, err, sql.ErrNoRows, "session should be deleted")

		// Verify cascade delete worked for session_player_race
		err = sqlH.Get(&sprCount, sprQuery)
		require.NoError(t, err)
		require.Equal(t, 0, sprCount, "session player races should be cascade deleted")

		// Verify cascade delete worked for ruleset
		err = sqlH.Get(&rulesetCount, rulesetQuery)
		require.NoError(t, err)
		require.Equal(t, 0, rulesetCount, "ruleset should be cascade deleted")
	})
}
