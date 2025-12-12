package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/fixtures"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
	"github.com/neper-stars/neper/sync"
)

func TestInvitationListHandler(t *testing.T) {
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
	listHandler := NewInvitationListHandler(&log, testdb.DB)

	t.Run("user_sees_only_their_own_invitations", func(t *testing.T) {
		sessionID := "gondorID"
		merryID := "merryID"
		gollumID := "gollumID"

		boromirPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "boromirID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// boromir invites merry to the gondor session
		invitationForMerry := models.Invitation{
			SessionID:     sessionID,
			UserProfileID: merryID,
		}
		paramsForMerry := operations.InvitationCreateParams{
			Invitation: &invitationForMerry,
			SessionID:  sessionID,
		}
		returnedInvForMerry, err := createHandler.handle(ctx, paramsForMerry, &boromirPrincipal)
		require.NoError(t, err)
		require.NotEmpty(t, returnedInvForMerry.ID)

		// boromir invites gollum to the gondor session
		invitationForGollum := models.Invitation{
			SessionID:     sessionID,
			UserProfileID: gollumID,
		}
		paramsForGollum := operations.InvitationCreateParams{
			Invitation: &invitationForGollum,
			SessionID:  sessionID,
		}
		returnedInvForGollum, err := createHandler.handle(ctx, paramsForGollum, &boromirPrincipal)
		require.NoError(t, err)
		require.NotEmpty(t, returnedInvForGollum.ID)

		// merry lists their invitations
		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   merryID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		merryInvitations, err := listHandler.handle(ctx, &merryPrincipal)
		require.NoError(t, err)
		require.Len(t, merryInvitations, 1)
		require.Equal(t, returnedInvForMerry.ID, merryInvitations[0].ID)
		require.Equal(t, "Gondor", merryInvitations[0].SessionName)
		require.Equal(t, "BoromirDúnedain", merryInvitations[0].InviterNickname)

		// gollum lists their invitations
		gollumPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   gollumID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		gollumInvitations, err := listHandler.handle(ctx, &gollumPrincipal)
		require.NoError(t, err)
		require.Len(t, gollumInvitations, 1)
		require.Equal(t, returnedInvForGollum.ID, gollumInvitations[0].ID)
		require.Equal(t, "Gondor", gollumInvitations[0].SessionName)
		require.Equal(t, "BoromirDúnedain", gollumInvitations[0].InviterNickname)

		// boromir (the inviter) should not see these invitations when listing
		boromirInvitations, err := listHandler.handle(ctx, &boromirPrincipal)
		require.NoError(t, err)
		require.Len(t, boromirInvitations, 0)
	})

	t.Run("user_with_no_invitations_sees_empty_list", func(t *testing.T) {
		finduilasPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "finduilasID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		invitations, err := listHandler.handle(ctx, &finduilasPrincipal)
		require.NoError(t, err)
		require.Len(t, invitations, 0)
	})
}
