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

func TestInvitationSentListHandler(t *testing.T) {
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

	createHandler := NewInvitationCreateHandler(&log, testdb.DB, nil)
	sentListHandler := NewInvitationSentListHandler(&log, testdb.DB)

	t.Run("inviter_sees_their_sent_invitations", func(t *testing.T) {
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

		// boromir lists their sent invitations
		sentInvitations, err := sentListHandler.handle(ctx, &boromirPrincipal)
		require.NoError(t, err)
		require.Len(t, sentInvitations, 2)

		// Verify the invitations have correct details
		invitationIDs := make(map[string]bool)
		for _, inv := range sentInvitations {
			invitationIDs[inv.ID] = true
			require.Equal(t, "Gondor", inv.SessionName)
			require.Equal(t, "boromirID", inv.InviterID)
			// Check invitee nickname is populated
			switch inv.UserProfileID {
			case merryID:
				require.Equal(t, "Merry", inv.InviteeNickname)
			case gollumID:
				require.Equal(t, "SméagolGollum", inv.InviteeNickname)
			}
		}
		require.True(t, invitationIDs[returnedInvForMerry.ID], "merry's invitation should be in the list")
		require.True(t, invitationIDs[returnedInvForGollum.ID], "gollum's invitation should be in the list")
	})

	t.Run("invitee_does_not_see_invitation_in_sent_list", func(t *testing.T) {
		// merry should not see invitations addressed to them in the sent list
		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		sentInvitations, err := sentListHandler.handle(ctx, &merryPrincipal)
		require.NoError(t, err)
		require.Len(t, sentInvitations, 0)
	})

	t.Run("user_with_no_sent_invitations_sees_empty_list", func(t *testing.T) {
		finduilasPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "finduilasID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		sentInvitations, err := sentListHandler.handle(ctx, &finduilasPrincipal)
		require.NoError(t, err)
		require.Len(t, sentInvitations, 0)
	})
}
