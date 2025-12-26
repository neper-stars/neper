package handlers

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/fixtures"
	"github.com/neper-stars/neper/lib/racefiles"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
	syncpkg "github.com/neper-stars/neper/sync"
	"github.com/neper-stars/neper/testutils/loghook"
)

func TestRaceCreateHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := syncpkg.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// load some sessions because our players are members of them
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	// humans are owned by Boromir
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	// hobbits are owned by Merry
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")

	handler := NewRaceCreateHandler(&log, testdb.DB, nil, nil)

	t.Run("boromir_creates_humans", func(t *testing.T) {
		rf, err := os.Open("fixtures/humans.r1")
		require.NoError(t, err)

		data, err := io.ReadAll(rf)
		require.NoError(t, err)

		dst := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
		base64.StdEncoding.Encode(dst, data)

		boromirID := "boromirID"

		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   boromirID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		race := models.Race{
			Data: string(dst),
		}

		params := operations.RaceCreateParams{
			UserProfileID: boromirID,
			Race:          &race,
		}

		returnedRace, err := handler.handle(
			ctx,
			params,
			&boromirPrincipal,
		)

		require.NoError(t, err)
		// Humans is a string that was set inside the race file during its creation
		// this proves our parser can successfully read race files
		require.Equal(t, "Humans", returnedRace.NamePlural)
		require.Equal(t, "Human", returnedRace.NameSingular)
		require.Equal(t, boromirID, returnedRace.UserID)
	})

	t.Run("corrupted_race_file_without_fix_enabled_logs_warning", func(t *testing.T) {
		// Create a logger with capture hook for assertions
		hook := &loghook.CaptureHook{}
		testLog := testutils.GetLogger(t).Hook(hook)

		// Create handler without fix enabled (processor with FixCorrupted=false)
		processor := racefiles.NewProcessor(racefiles.ProcessorOptions{
			StripPassword: false,
			FixCorrupted:  false,
		})
		handlerNoFix := NewRaceCreateHandler(&testLog, testdb.DB, processor, nil)

		rf, err := os.Open("fixtures/needsrepair.r1")
		require.NoError(t, err)
		defer func() { _ = rf.Close() }()

		data, err := io.ReadAll(rf)
		require.NoError(t, err)

		dst := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
		base64.StdEncoding.Encode(dst, data)

		boromirID := "boromirID"

		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   boromirID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		race := models.Race{
			Data: string(dst),
		}

		params := operations.RaceCreateParams{
			UserProfileID: boromirID,
			Race:          &race,
		}

		// Race should still be created (corruption doesn't block creation)
		returnedRace, err := handlerNoFix.handle(ctx, params, &boromirPrincipal)
		require.NoError(t, err)
		require.NotEmpty(t, returnedRace.ID)
		require.Equal(t, boromirID, returnedRace.UserID)
		// The race data should be unchanged (not repaired)
		require.Equal(t, string(dst), returnedRace.Data)

		// Assert that a warning was logged about corruption
		require.True(t, hook.HasMessage(zerolog.WarnLevel, "race file is corrupted but automatic repair is disabled"),
			"expected warning about corrupted race file")
	})

	t.Run("corrupted_race_file_with_fix_enabled_repairs_file", func(t *testing.T) {
		// Create a logger with capture hook for assertions
		hook := &loghook.CaptureHook{}
		testLog := testutils.GetLogger(t).Hook(hook)

		// Create handler with fix enabled
		processor := racefiles.NewProcessor(racefiles.ProcessorOptions{
			StripPassword: false,
			FixCorrupted:  true,
		})
		handlerWithFix := NewRaceCreateHandler(&testLog, testdb.DB, processor, nil)

		rf, err := os.Open("fixtures/needsrepair.r1")
		require.NoError(t, err)
		defer func() { _ = rf.Close() }()

		data, err := io.ReadAll(rf)
		require.NoError(t, err)

		originalB64 := base64.StdEncoding.EncodeToString(data)

		boromirID := "boromirID"

		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   boromirID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		race := models.Race{
			Data: originalB64,
		}

		params := operations.RaceCreateParams{
			UserProfileID: boromirID,
			Race:          &race,
		}

		// Race should be created and repaired
		returnedRace, err := handlerWithFix.handle(ctx, params, &boromirPrincipal)
		require.NoError(t, err)
		require.NotEmpty(t, returnedRace.ID)
		require.Equal(t, boromirID, returnedRace.UserID)
		// The race data should be different (repaired)
		require.NotEqual(t, originalB64, returnedRace.Data, "race file should have been repaired and data changed")

		// Assert that an info message was logged about successful repair
		require.True(t, hook.HasMessage(zerolog.InfoLevel, "corrupted race file was automatically repaired"),
			"expected info message about race file repair")
	})
}
