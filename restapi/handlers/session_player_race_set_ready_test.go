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

func TestSessionPlayerRaceSetReadyHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	// Load sessions, members, and races
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/races.json")

	sprCreateHandler := NewSessionPlayerRaceCreateHandler(&log, testdb.DB)
	setReadyHandler := NewSessionPlayerRaceSetReadyHandler(&log, testdb.DB)

	t.Run("user_can_set_their_own_ready_status", func(t *testing.T) {
		sessionID := "gondorID"
		boromirID := "boromirID"

		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   boromirID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// First, register boromir's race for the session
		sprCreateParams := operations.SessionPlayerRaceCreateParams{
			SessionID: sessionID,
			SessionPlayerRace: &models.SessionPlayerRace{
				RaceID:    "humansID",
				SessionID: sessionID,
			},
		}
		spr, err := sprCreateHandler.handle(ctx, sprCreateParams, &boromirPrincipal)
		require.NoError(t, err)
		require.False(t, spr.Ready, "ready should be false initially")

		// Now set ready to true
		setReadyParams := operations.SessionPlayerRaceSetReadyParams{
			SessionID: sessionID,
			Ready:     true,
		}
		updatedSpr, err := setReadyHandler.handle(ctx, setReadyParams, &boromirPrincipal)
		require.NoError(t, err)
		require.True(t, updatedSpr.Ready, "ready should be true after setting")

		// Set ready back to false
		setReadyParams.Ready = false
		updatedSpr, err = setReadyHandler.handle(ctx, setReadyParams, &boromirPrincipal)
		require.NoError(t, err)
		require.False(t, updatedSpr.Ready, "ready should be false after unsetting")
	})

	t.Run("user_cannot_set_another_users_ready_status", func(t *testing.T) {
		sessionID := "gondorID"
		finduilasID := "finduilasID"
		boromirID := "boromirID"

		finduilasPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   finduilasID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Finduilas tries to set ready status, but boromir has the race registered
		// Finduilas doesn't have a race registered, so she gets not found
		setReadyParams := operations.SessionPlayerRaceSetReadyParams{
			SessionID: sessionID,
			Ready:     true,
		}
		_, err := setReadyHandler.handle(ctx, setReadyParams, &finduilasPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrNotFound), "should get not found since finduilas has no race registered")

		// Let's verify boromir's ready status is still what we expect (from previous test)
		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   boromirID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Boromir can still access his own
		setReadyParams = operations.SessionPlayerRaceSetReadyParams{
			SessionID: sessionID,
			Ready:     true,
		}
		updatedSpr, err := setReadyHandler.handle(ctx, setReadyParams, &boromirPrincipal)
		require.NoError(t, err)
		require.True(t, updatedSpr.Ready)
	})

	t.Run("global_manager_can_set_own_ready_status", func(t *testing.T) {
		sessionID := "gondorID"
		boromirID := "boromirID"

		adminPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   boromirID, // Using boromir's ID but as global manager
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		// Global manager can modify the ready status of his own player (not others)
		setReadyParams := operations.SessionPlayerRaceSetReadyParams{
			SessionID: sessionID,
			Ready:     false,
		}
		updatedSpr, err := setReadyHandler.handle(ctx, setReadyParams, &adminPrincipal)
		require.NoError(t, err)
		require.False(t, updatedSpr.Ready)
	})

	t.Run("setting_ready_on_nonexistent_session_returns_not_found", func(t *testing.T) {
		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		setReadyParams := operations.SessionPlayerRaceSetReadyParams{
			SessionID: "nonexistent-session",
			Ready:     true,
		}
		_, err := setReadyHandler.handle(ctx, setReadyParams, &boromirPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrNotFound))
	})
}
