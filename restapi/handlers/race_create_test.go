package handlers

import (
	"context"
	"encoding/base64"
	"errors"
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
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/race"
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

	handler := NewRaceCreateHandler(&log, testdb.DB, nil, nil, nil)

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
		handlerNoFix := NewRaceCreateHandler(&testLog, testdb.DB, processor, nil, nil)

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
		handlerWithFix := NewRaceCreateHandler(&testLog, testdb.DB, processor, nil, nil)

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

func TestRaceCreateHandler_Limits(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	sqlH := database.NewSQLHelper(ctx, testdb.DB, log)

	// Load a race file for testing
	rf, err := os.Open("fixtures/humans.r1")
	require.NoError(t, err)
	defer func() { _ = rf.Close() }()
	data, err := io.ReadAll(rf)
	require.NoError(t, err)
	raceData := base64.StdEncoding.EncodeToString(data)

	t.Run("pending_user_limit", func(t *testing.T) {
		// Create handler with limit of 2 for pending users
		opts := &race.Options{
			PendingUserLimit:  2,
			ApprovedUserLimit: 50,
		}
		handler := NewRaceCreateHandler(&log, testdb.DB, nil, nil, opts)

		pendingUserID := "pending-user-limit-test"

		// Create the user profile first
		_, err := sqlH.Insert(&models.UserProfileDB{
			UserProfile: models.UserProfile{
				ID:       pendingUserID,
				Nickname: "PendingUser",
				Email:    "pending@test.com",
				IsActive: true,
				Pending:  true,
			},
		})
		require.NoError(t, err)

		pendingPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   pendingUserID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
			IsPending:       true,
		}

		// First 2 races should succeed
		for i := 0; i < 2; i++ {
			params := operations.RaceCreateParams{
				UserProfileID: pendingUserID,
				Race:          &models.Race{Data: raceData},
			}
			_, err := handler.handle(ctx, params, &pendingPrincipal)
			require.NoError(t, err, "race %d should succeed", i+1)
		}

		// Third race should fail
		params := operations.RaceCreateParams{
			UserProfileID: pendingUserID,
			Race:          &models.Race{Data: raceData},
		}
		_, err = handler.handle(ctx, params, &pendingPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrRaceLimitExceeded), "should get race limit exceeded error")
	})

	t.Run("approved_user_has_higher_limit", func(t *testing.T) {
		// Create handler with limit of 1 for pending, 3 for approved
		opts := &race.Options{
			PendingUserLimit:  1,
			ApprovedUserLimit: 3,
		}
		handler := NewRaceCreateHandler(&log, testdb.DB, nil, nil, opts)

		approvedUserID := "approved-user-limit-test"

		// Create the user profile first
		_, err := sqlH.Insert(&models.UserProfileDB{
			UserProfile: models.UserProfile{
				ID:       approvedUserID,
				Nickname: "ApprovedUser",
				Email:    "approved@test.com",
				IsActive: true,
				Pending:  false,
			},
		})
		require.NoError(t, err)

		approvedPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   approvedUserID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
			IsPending:       false,
		}

		// First 3 races should succeed for approved user
		for i := 0; i < 3; i++ {
			params := operations.RaceCreateParams{
				UserProfileID: approvedUserID,
				Race:          &models.Race{Data: raceData},
			}
			_, err := handler.handle(ctx, params, &approvedPrincipal)
			require.NoError(t, err, "race %d should succeed", i+1)
		}

		// Fourth race should fail
		params := operations.RaceCreateParams{
			UserProfileID: approvedUserID,
			Race:          &models.Race{Data: raceData},
		}
		_, err = handler.handle(ctx, params, &approvedPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrRaceLimitExceeded), "should get race limit exceeded error")
	})

	t.Run("no_limit_when_opts_nil", func(t *testing.T) {
		// Create handler without options
		handler := NewRaceCreateHandler(&log, testdb.DB, nil, nil, nil)

		userID := "no-limit-test"

		// Create the user profile first
		_, err := sqlH.Insert(&models.UserProfileDB{
			UserProfile: models.UserProfile{
				ID:       userID,
				Nickname: "NoLimitUser",
				Email:    "nolimit@test.com",
				IsActive: true,
				Pending:  true,
			},
		})
		require.NoError(t, err)

		principal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   userID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
			IsPending:       true,
		}

		// Should be able to create many races without limit
		for i := 0; i < 5; i++ {
			params := operations.RaceCreateParams{
				UserProfileID: userID,
				Race:          &models.Race{Data: raceData},
			}
			_, err := handler.handle(ctx, params, &principal)
			require.NoError(t, err, "race %d should succeed", i+1)
		}
	})
}
