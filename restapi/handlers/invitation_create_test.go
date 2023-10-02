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

func TestInvitationCreateHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// load some sessions because our players are members of them
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	// humans are owned by Boromir
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	// hobbits are owned by Merry
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")

	handler := NewInvitationCreateHandler(&log, testdb.DB)

	t.Run("boromir_invites_merry_to_gondor_session", func(t *testing.T) {
		sessionID := "gondorID"
		// invite merry to the humans session
		invitation := models.Invitation{
			SessionID:     sessionID,
			UserProfileID: "merryID",
		}

		params := operations.SessionInviteParams{
			Invitation: &invitation,
			SessionID:  sessionID,
		}

		returnedInvitation, err := handler.handle(
			ctx,
			params,
			&models.Principal{
				StandardClaims: jwt.StandardClaims{
					Subject:   "boromirID",
					ExpiresAt: time.Now().Add(time.Minute).Unix(),
				},
				IsGlobalManager: false,
			},
		)
		require.NoError(t, err)
		require.Equal(t, "gondorID", returnedInvitation.SessionID)
		require.Equal(t, "merryID", returnedInvitation.UserProfileID)

		// try again and get an error because you can invite the same
		// person twice on the same session
		_, err = handler.handle(
			ctx,
			params,
			&models.Principal{
				StandardClaims: jwt.StandardClaims{
					Subject:   "boromirID",
					ExpiresAt: time.Now().Add(time.Minute).Unix(),
				},
				IsGlobalManager: false,
			},
		)
		require.Error(t, err)
		require.Equal(t, "invitation already exists for user: boromirIDand session: gondorID", err.Error())
	})

	t.Run("boromir_invites_merry_to_shire_session_no_authorized", func(t *testing.T) {
		sessionID := "shireID"
		// invite merry to the shire session
		invitation := models.Invitation{
			SessionID:     sessionID,
			UserProfileID: "merryID",
		}

		params := operations.SessionInviteParams{
			Invitation: &invitation,
			SessionID:  sessionID,
		}
		_, err := handler.handle(
			ctx,
			params,
			&models.Principal{
				StandardClaims: jwt.StandardClaims{
					Subject:   "boromirID",
					ExpiresAt: time.Now().Add(time.Minute).Unix(),
				},
				IsGlobalManager: false,
			},
		)
		require.Error(t, err)
		require.Equal(t, "forbidden", err.Error())
	})
	t.Run("boromir_uses_different_session_id_in_path_and_body_forbidden", func(t *testing.T) {
		sessionID := "gondorID"
		otherSessionID := "shireID"
		// invite merry to the shire session
		invitation := models.Invitation{
			SessionID:     otherSessionID, // <-- in the body we use a session we are NOT allowed (not manager)
			UserProfileID: "merryID",
		}

		params := operations.SessionInviteParams{
			Invitation: &invitation,
			SessionID:  sessionID, // <-- in the path we use a session we are allowed
		}
		_, err := handler.handle(
			ctx,
			params,
			&models.Principal{
				StandardClaims: jwt.StandardClaims{
					Subject:   "boromirID",
					ExpiresAt: time.Now().Add(time.Minute).Unix(),
				},
				IsGlobalManager: false,
			},
		)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden)) // <-- nice! the system detected our little trick
	})
}
