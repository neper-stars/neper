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

func TestSessionArchiveHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_started.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")

	archiveHandler := NewSessionArchiveHandler(&log, testdb.DB, nil)

	t.Run("session_manager_can_archive_started_session", func(t *testing.T) {
		sessionID := "gondorID"

		// boromir is a manager of gondor
		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		archiveParams := operations.SessionArchiveParams{
			SessionID: sessionID,
		}
		session, err := archiveHandler.handle(ctx, archiveParams, &boromirPrincipal)
		require.NoError(t, err)
		require.Equal(t, models.SessionStateArchived, session.State)

		// Verify in database
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var sessionDB models.SessionDB
		require.NoError(t, sqlH.GetByPKey(&sessionDB, sessionID))
		require.Equal(t, models.SessionStateArchived, sessionDB.State)
	})
}

func TestSessionArchiveHandler_GlobalManager(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_started.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")

	archiveHandler := NewSessionArchiveHandler(&log, testdb.DB, nil)

	t.Run("global_manager_can_archive_any_started_session", func(t *testing.T) {
		sessionID := "gondorID"

		// merry is not a member of gondor but is a global manager
		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		archiveParams := operations.SessionArchiveParams{
			SessionID: sessionID,
		}
		session, err := archiveHandler.handle(ctx, archiveParams, &merryPrincipal)
		require.NoError(t, err)
		require.Equal(t, models.SessionStateArchived, session.State)
	})
}

func TestSessionArchiveHandler_NonManagerForbidden(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_started.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")

	archiveHandler := NewSessionArchiveHandler(&log, testdb.DB, nil)

	t.Run("regular_member_cannot_archive_session", func(t *testing.T) {
		sessionID := "gondorID"

		// finduilas is a regular member (not manager) of gondor
		finduilasPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "finduilasID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		archiveParams := operations.SessionArchiveParams{
			SessionID: sessionID,
		}
		_, err := archiveHandler.handle(ctx, archiveParams, &finduilasPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden))
	})

	t.Run("non_member_cannot_archive_session", func(t *testing.T) {
		// Load merry who is not a member of gondor
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")

		sessionID := "gondorID"

		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		archiveParams := operations.SessionArchiveParams{
			SessionID: sessionID,
		}
		_, err := archiveHandler.handle(ctx, archiveParams, &merryPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden))
	})
}

func TestSessionArchiveHandler_PendingSessionCannotBeArchived(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// Load pending session (not started)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")

	archiveHandler := NewSessionArchiveHandler(&log, testdb.DB, nil)

	t.Run("cannot_archive_pending_session", func(t *testing.T) {
		sessionID := "gondorID"

		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		archiveParams := operations.SessionArchiveParams{
			SessionID: sessionID,
		}
		_, err := archiveHandler.handle(ctx, archiveParams, &boromirPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrPreconditionFailed))
		require.Contains(t, err.Error(), "started")
	})
}

func TestSessionArchiveHandler_AlreadyArchivedSession(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_started.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")

	archiveHandler := NewSessionArchiveHandler(&log, testdb.DB, nil)

	sessionID := "gondorID"

	boromirPrincipal := models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "boromirID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		IsGlobalManager: false,
	}

	// First archive should succeed
	archiveParams := operations.SessionArchiveParams{
		SessionID: sessionID,
	}
	_, err = archiveHandler.handle(ctx, archiveParams, &boromirPrincipal)
	require.NoError(t, err)

	// Second archive should fail
	t.Run("cannot_archive_already_archived_session", func(t *testing.T) {
		_, err := archiveHandler.handle(ctx, archiveParams, &boromirPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrPreconditionFailed))
		require.Contains(t, err.Error(), "started")
	})
}

func TestSessionArchiveHandler_NonexistentSession(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	archiveHandler := NewSessionArchiveHandler(&log, testdb.DB, nil)

	t.Run("archive_nonexistent_session_returns_not_found", func(t *testing.T) {
		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		archiveParams := operations.SessionArchiveParams{
			SessionID: "nonexistent-session-id",
		}
		_, err := archiveHandler.handle(ctx, archiveParams, &boromirPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrNotFound))
	})
}

func TestSessionArchiveHandler_PendingUserCannotArchive(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_started.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")

	archiveHandler := NewSessionArchiveHandler(&log, testdb.DB, nil)

	t.Run("pending_user_cannot_archive_session", func(t *testing.T) {
		sessionID := "gondorID"

		// boromir is a manager but has pending status
		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
			IsPending:       true,
		}

		archiveParams := operations.SessionArchiveParams{
			SessionID: sessionID,
		}
		_, err := archiveHandler.handle(ctx, archiveParams, &boromirPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden))
	})
}
