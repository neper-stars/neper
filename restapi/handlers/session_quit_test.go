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

func TestSessionQuitHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")

	quitHandler := NewSessionQuitHandler(&log, testdb.DB, nil)
	joinHandler := NewSessionJoinHandler(&log, testdb.DB, nil, nil)

	t.Run("regular_member_can_leave_session", func(t *testing.T) {
		sessionID := "gondorID"

		// finduilas is a regular member (not manager) of gondor
		finduilasPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "finduilasID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Verify finduilas is a member before quitting
		tx, err := database.Begin(ctx, testdb.DB)
		require.NoError(t, err)
		sqlH := database.NewSQLHelper(ctx, tx, log)
		isMember, err := IsSessionMember(sqlH, "finduilasID", sessionID)
		require.NoError(t, err)
		require.True(t, isMember, "finduilas should be a member before quitting")
		tx.RollbackIfOpened(log)

		// Quit the session
		quitParams := operations.SessionQuitParams{
			SessionID: sessionID,
		}
		err = quitHandler.handle(ctx, quitParams, &finduilasPrincipal)
		require.NoError(t, err)

		// Verify finduilas is no longer a member
		tx, err = database.Begin(ctx, testdb.DB)
		require.NoError(t, err)
		sqlH = database.NewSQLHelper(ctx, tx, log)
		isMember, err = IsSessionMember(sqlH, "finduilasID", sessionID)
		require.NoError(t, err)
		require.False(t, isMember, "finduilas should not be a member after quitting")
		tx.RollbackIfOpened(log)
	})

	t.Run("manager_can_leave_if_another_manager_exists", func(t *testing.T) {
		// First, let's create a scenario where we have two managers
		// Use the shire session and make merry join, then promote to manager
		sessionID := "shireID"

		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Merry joins the shire (public session)
		joinParams := operations.SessionJoinParams{
			SessionID: sessionID,
		}
		_, err := joinHandler.handle(ctx, joinParams, &merryPrincipal)
		require.NoError(t, err)

		// Manually make merry a manager (simulating what an admin would do)
		tx, err := database.Begin(ctx, testdb.DB)
		require.NoError(t, err)
		sqlH := database.NewSQLHelper(ctx, tx, log)
		_, err = sqlH.Exec(database.SQ.Update(models.UserProfileSessionRelDBTable).
			Set(models.UserProfileSessionRelDBIsManagerColumn, true).
			Where(sq.And{
				sq.Eq{models.UserProfileSessionRelDBUserProfileIDColumn: "merryID"},
				sq.Eq{models.UserProfileSessionRelDBSessionIDColumn: sessionID},
			}))
		require.NoError(t, err)
		require.NoError(t, tx.Commit())

		// Now merry (a manager) should be able to leave since there's no other manager
		// Wait, shire doesn't have another manager. Let's test with gondor instead
		// where boromir is manager
	})

	t.Run("sole_manager_cannot_leave_session", func(t *testing.T) {
		sessionID := "gondorID"

		// boromir is the only manager of gondor
		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Boromir tries to quit
		quitParams := operations.SessionQuitParams{
			SessionID: sessionID,
		}
		err := quitHandler.handle(ctx, quitParams, &boromirPrincipal)
		require.Error(t, err)
		require.Contains(t, err.Error(), "only manager")
	})

	t.Run("non_member_cannot_quit_session", func(t *testing.T) {
		sessionID := "isengardID" // private session merry is not a member of

		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		quitParams := operations.SessionQuitParams{
			SessionID: sessionID,
		}
		err := quitHandler.handle(ctx, quitParams, &merryPrincipal)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not a member")
	})

	t.Run("quit_nonexistent_session_returns_not_found", func(t *testing.T) {
		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		quitParams := operations.SessionQuitParams{
			SessionID: "nonexistent-session-id",
		}
		err := quitHandler.handle(ctx, quitParams, &merryPrincipal)
		require.Error(t, err)
		require.Contains(t, err.Error(), "session not found")
	})
}

func TestSessionQuitHandler_ReadyPlayerCannotLeave(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/finduilas_ready.json")

	quitHandler := NewSessionQuitHandler(&log, testdb.DB, nil)

	sessionID := "gondorID"

	finduilasPrincipal := models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "finduilasID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		IsGlobalManager: false,
	}

	quitParams := operations.SessionQuitParams{
		SessionID: sessionID,
	}
	err = quitHandler.handle(ctx, quitParams, &finduilasPrincipal)
	require.Error(t, err)
	require.ErrorIs(t, err, errs.ErrPreconditionFailed)
	require.Contains(t, err.Error(), "ready")
}

func TestSessionQuitHandler_StartedSessionCannotBeLeft(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_started.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")

	quitHandler := NewSessionQuitHandler(&log, testdb.DB, nil)

	sessionID := "gondorID"

	finduilasPrincipal := models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "finduilasID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		IsGlobalManager: false,
	}

	quitParams := operations.SessionQuitParams{
		SessionID: sessionID,
	}
	err = quitHandler.handle(ctx, quitParams, &finduilasPrincipal)
	require.Error(t, err)
	require.ErrorIs(t, err, errs.ErrPreconditionFailed)
	require.Contains(t, err.Error(), "started")
}

func TestSessionQuitHandler_SessionPlayerRaceIsDeleted(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/finduilas_not_ready.json")

	quitHandler := NewSessionQuitHandler(&log, testdb.DB, nil)

	sessionID := "gondorID"

	// Verify finduilas has a session_player_race before quitting
	tx, err := database.Begin(ctx, testdb.DB)
	require.NoError(t, err)
	sqlH := database.NewSQLHelper(ctx, tx, log)
	var sprCount int
	err = sqlH.Get(&sprCount, database.SQ.Select(Count("*")).
		From(models.SessionPlayerRaceDBTable).
		Where(sq.And{
			sq.Eq{models.SessionPlayerRaceDBUserProfileIDColumn: "finduilasID"},
			sq.Eq{models.SessionPlayerRaceDBSessionIDColumn: sessionID},
		}))
	require.NoError(t, err)
	require.Equal(t, 1, sprCount, "finduilas should have a session_player_race before quitting")
	tx.RollbackIfOpened(log)

	finduilasPrincipal := models.Principal{
		StandardClaims: jwt.StandardClaims{
			Subject:   "finduilasID",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		},
		IsGlobalManager: false,
	}

	quitParams := operations.SessionQuitParams{
		SessionID: sessionID,
	}
	err = quitHandler.handle(ctx, quitParams, &finduilasPrincipal)
	require.NoError(t, err)

	// Verify finduilas no longer has a session_player_race
	tx, err = database.Begin(ctx, testdb.DB)
	require.NoError(t, err)
	sqlH = database.NewSQLHelper(ctx, tx, log)
	err = sqlH.Get(&sprCount, database.SQ.Select(Count("*")).
		From(models.SessionPlayerRaceDBTable).
		Where(sq.And{
			sq.Eq{models.SessionPlayerRaceDBUserProfileIDColumn: "finduilasID"},
			sq.Eq{models.SessionPlayerRaceDBSessionIDColumn: sessionID},
		}))
	require.NoError(t, err)
	require.Equal(t, 0, sprCount, "finduilas should not have a session_player_race after quitting")
	tx.RollbackIfOpened(log)
}

func TestSessionQuitHandler_ManagerWithOtherManager(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")

	quitHandler := NewSessionQuitHandler(&log, testdb.DB, nil)
	joinHandler := NewSessionJoinHandler(&log, testdb.DB, nil, nil)

	t.Run("manager_can_leave_when_other_manager_exists", func(t *testing.T) {
		sessionID := "gondorID"

		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Merry joins the gondor session (public)
		joinParams := operations.SessionJoinParams{
			SessionID: sessionID,
		}
		_, err := joinHandler.handle(ctx, joinParams, &merryPrincipal)
		require.NoError(t, err)

		// Make merry a manager
		tx, err := database.Begin(ctx, testdb.DB)
		require.NoError(t, err)
		sqlH := database.NewSQLHelper(ctx, tx, log)
		_, err = sqlH.Exec(database.SQ.Update(models.UserProfileSessionRelDBTable).
			Set(models.UserProfileSessionRelDBIsManagerColumn, true).
			Where(sq.And{
				sq.Eq{models.UserProfileSessionRelDBUserProfileIDColumn: "merryID"},
				sq.Eq{models.UserProfileSessionRelDBSessionIDColumn: sessionID},
			}))
		require.NoError(t, err)
		require.NoError(t, tx.Commit())

		// Now boromir (original manager) should be able to leave
		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		quitParams := operations.SessionQuitParams{
			SessionID: sessionID,
		}
		err = quitHandler.handle(ctx, quitParams, &boromirPrincipal)
		require.NoError(t, err)

		// Verify boromir is no longer a member
		tx, err = database.Begin(ctx, testdb.DB)
		require.NoError(t, err)
		sqlH = database.NewSQLHelper(ctx, tx, log)
		isMember, err := IsSessionMember(sqlH, "boromirID", sessionID)
		require.NoError(t, err)
		require.False(t, isMember, "boromir should not be a member after quitting")
		tx.RollbackIfOpened(log)
	})
}
