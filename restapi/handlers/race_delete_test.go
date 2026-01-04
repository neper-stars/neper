package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
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

func TestRaceDeleteHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/races.json")

	createHandler := NewRaceCreateHandler(&log, testdb.DB, nil, nil, nil)
	deleteHandler := NewRaceDeleteHandler(testdb.DB, nil)
	sessionPlayerRaceHandler := NewSessionPlayerRaceCreateHandler(&log, testdb.DB, nil)

	t.Run("owner_can_delete_their_own_race", func(t *testing.T) {
		// First create a race
		rf, err := os.Open("fixtures/humans.r1")
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
			Data:   string(dst),
			UserID: boromirID,
		}

		createParams := operations.RaceCreateParams{
			UserProfileID: boromirID,
			Race:          &race,
		}

		returnedRace, err := createHandler.handle(ctx, createParams, &boromirPrincipal)
		require.NoError(t, err)
		require.NotEmpty(t, returnedRace.ID)

		// Now delete the race
		deleteParams := operations.RaceDeleteParams{
			UserProfileID: boromirID,
			RaceID:        returnedRace.ID,
		}

		err = deleteHandler.handle(ctx, deleteParams, &boromirPrincipal)
		require.NoError(t, err)

		// Verify race is removed from DB
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var count int
		err = sqlH.Get(&count, database.SQ.Select("COUNT(*)").From(models.RaceDBTable).Where(sq.Eq{models.RaceDBIDColumn: returnedRace.ID}))
		require.NoError(t, err)
		require.Equal(t, 0, count)
	})

	t.Run("user_cannot_delete_race_belonging_to_someone_else", func(t *testing.T) {
		// First create a race as boromir
		rf, err := os.Open("fixtures/humans.r1")
		require.NoError(t, err)
		defer func() { _ = rf.Close() }()

		data, err := io.ReadAll(rf)
		require.NoError(t, err)

		dst := make([]byte, base64.StdEncoding.EncodedLen(len(data)))
		base64.StdEncoding.Encode(dst, data)

		boromirID := "boromirID"
		merryID := "merryID"

		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   boromirID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		race := models.Race{
			Data:   string(dst),
			UserID: boromirID,
		}

		createParams := operations.RaceCreateParams{
			UserProfileID: boromirID,
			Race:          &race,
		}

		returnedRace, err := createHandler.handle(ctx, createParams, &boromirPrincipal)
		require.NoError(t, err)

		// merry tries to delete boromir's race
		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   merryID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		deleteParams := operations.RaceDeleteParams{
			UserProfileID: boromirID,
			RaceID:        returnedRace.ID,
		}

		err = deleteHandler.handle(ctx, deleteParams, &merryPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden))
	})

	t.Run("deleting_nonexistent_race_returns_not_found", func(t *testing.T) {
		boromirID := "boromirID"

		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   boromirID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true, // use global manager to bypass ownership check
		}
		deleteParams := operations.RaceDeleteParams{
			UserProfileID: boromirID,
			RaceID:        "nonexistent-race-id",
		}

		err := deleteHandler.handle(ctx, deleteParams, &boromirPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrNotFound))
	})

	t.Run("global_manager_can_delete_any_race", func(t *testing.T) {
		// First create a race as boromir
		rf, err := os.Open("fixtures/humans.r1")
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
			Data:   string(dst),
			UserID: boromirID,
		}

		createParams := operations.RaceCreateParams{
			UserProfileID: boromirID,
			Race:          &race,
		}

		returnedRace, err := createHandler.handle(ctx, createParams, &boromirPrincipal)
		require.NoError(t, err)

		// global manager deletes the race
		adminPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "adminID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}
		deleteParams := operations.RaceDeleteParams{
			UserProfileID: boromirID,
			RaceID:        returnedRace.ID,
		}

		err = deleteHandler.handle(ctx, deleteParams, &adminPrincipal)
		require.NoError(t, err)

		// Verify race is removed from DB
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var count int
		err = sqlH.Get(&count, database.SQ.Select("COUNT(*)").From(models.RaceDBTable).Where(sq.Eq{models.RaceDBIDColumn: returnedRace.ID}))
		require.NoError(t, err)
		require.Equal(t, 0, count)
	})

	t.Run("cannot_delete_race_that_is_in_use_in_session", func(t *testing.T) {
		// Use the pre-loaded "humansID" race from fixtures/races.json
		boromirID := "boromirID"

		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   boromirID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Register boromir's race for the gondor session
		sprCreateParams := operations.SessionPlayerRaceCreateParams{
			SessionID: "gondorID",
			SessionPlayerRace: &models.SessionPlayerRace{
				IsBot:         false,
				PlayerOrder:   0,
				RaceID:        "humansID",
				SessionID:     "gondorID",
				UserProfileID: boromirID,
			},
		}
		_, err := sessionPlayerRaceHandler.handle(ctx, sprCreateParams, &boromirPrincipal)
		require.NoError(t, err)

		// Now try to delete the race that is in use
		deleteParams := operations.RaceDeleteParams{
			UserProfileID: boromirID,
			RaceID:        "humansID",
		}

		err = deleteHandler.handle(ctx, deleteParams, &boromirPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrConflict))
		require.Contains(t, err.Error(), "race is in use")
	})
}
