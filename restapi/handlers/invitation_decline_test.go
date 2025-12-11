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

func TestInvitationDeclineHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gondor_members.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merry_nosession.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/gollum_nosession.json")

	createHandler := NewInvitationCreateHandler(&log, testdb.DB)
	declineHandler := NewInvitationDeclineHandler(&log, testdb.DB)

	t.Run("invitee_can_decline_their_own_invitation", func(t *testing.T) {
		sessionID := "gondorID"
		merryID := "merryID"

		// boromir invites merry to the gondor session
		invitation := models.Invitation{
			SessionID:     sessionID,
			UserProfileID: merryID,
		}

		params := operations.InvitationCreateParams{
			Invitation: &invitation,
			SessionID:  sessionID,
		}

		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		returnedInvitation, err := createHandler.handle(ctx, params, &boromirPrincipal)
		require.NoError(t, err)
		require.NotEmpty(t, returnedInvitation.ID)

		// merry declines the invitation
		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   merryID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		declineParams := operations.InvitationDeclineParams{
			InvitationID: returnedInvitation.ID,
		}
		err = declineHandler.handle(ctx, declineParams, &merryPrincipal)
		require.NoError(t, err)

		// verify invitation is removed from DB
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var count int
		err = sqlH.Get(&count, sq.Select("COUNT(*)").From(models.InvitationDBTable).Where(sq.Eq{models.InvitationDBIDColumn: returnedInvitation.ID}))
		require.NoError(t, err)
		require.Equal(t, 0, count)
	})

	t.Run("user_cannot_decline_invitation_addressed_to_someone_else", func(t *testing.T) {
		sessionID := "gondorID"
		merryID := "merryID"
		gollumID := "gollumID"

		// boromir invites merry to the gondor session
		invitation := models.Invitation{
			SessionID:     sessionID,
			UserProfileID: merryID,
		}

		params := operations.InvitationCreateParams{
			Invitation: &invitation,
			SessionID:  sessionID,
		}

		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		returnedInvitation, err := createHandler.handle(ctx, params, &boromirPrincipal)
		require.NoError(t, err)

		// gollum tries to decline merry's invitation
		gollumPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   gollumID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		declineParams := operations.InvitationDeclineParams{
			InvitationID: returnedInvitation.ID,
		}
		err = declineHandler.handle(ctx, declineParams, &gollumPrincipal)
		require.ErrorIs(t, err, errs.ErrForbidden)
	})

	t.Run("declining_nonexistent_invitation_returns_not_found", func(t *testing.T) {
		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true, // use global manager to bypass ownership check
		}
		declineParams := operations.InvitationDeclineParams{
			InvitationID: "nonexistent-invitation-id",
		}
		err := declineHandler.handle(ctx, declineParams, &merryPrincipal)
		require.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("global_manager_can_decline_any_invitation", func(t *testing.T) {
		sessionID := "shireID"
		gollumID := "gollumID"

		// boromir invites gollum to the shire session
		invitation := models.Invitation{
			SessionID:     sessionID,
			UserProfileID: gollumID,
		}

		params := operations.InvitationCreateParams{
			Invitation: &invitation,
			SessionID:  sessionID,
		}

		// use global manager to create invitation
		adminPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		returnedInvitation, err := createHandler.handle(ctx, params, &adminPrincipal)
		require.NoError(t, err)

		// global manager (not gollum) declines the invitation
		declineParams := operations.InvitationDeclineParams{
			InvitationID: returnedInvitation.ID,
		}
		err = declineHandler.handle(ctx, declineParams, &adminPrincipal)
		require.NoError(t, err)

		// verify invitation is removed from DB
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var count int
		err = sqlH.Get(&count, sq.Select("COUNT(*)").From(models.InvitationDBTable).Where(sq.Eq{models.InvitationDBIDColumn: returnedInvitation.ID}))
		require.NoError(t, err)
		require.Equal(t, 0, count)
	})
}
