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
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
	"github.com/neper-stars/neper/sync"
)

func TestInvitationAcceptHandler(t *testing.T) {
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

	createHandler := NewInvitationCreateHandler(&log, testdb.DB, nil)
	acceptHandler := NewInvitationAcceptHandler(&log, testdb.DB, nil)

	t.Run("boromir_invites_merry_to_gondor_session", func(t *testing.T) {
		sessionID := "gondorID"
		merryID := "merryID"
		// invite merry to the humans session
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
		require.Equal(t, sessionID, returnedInvitation.SessionID)
		require.Equal(t, merryID, returnedInvitation.UserProfileID)
		// verify new fields
		require.Equal(t, "boromirID", returnedInvitation.InviterID)
		require.Equal(t, "Gondor", returnedInvitation.SessionName)
		require.Equal(t, "BoromirDúnedain", returnedInvitation.InviterNickname)

		// and now merry accepts invitation
		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   merryID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}
		acceptParams := operations.InvitationAcceptParams{
			InvitationID: returnedInvitation.ID,
		}
		returnedSession, err := acceptHandler.handle(ctx, acceptParams, &merryPrincipal)
		require.NoError(t, err)
		require.Equal(t, sessionID, returnedSession.ID)
		// finduilas & merry should now be in the members list
		require.Equal(t, 2, len(returnedSession.Members))
		var merryIsMember bool
		for _, memberID := range returnedSession.Members {
			if merryID == memberID {
				merryIsMember = true
			}
		}
		require.True(t, merryIsMember)

		// verify invitation is removed from DB
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var count int
		err = sqlH.Get(&count, database.SQ.Select("COUNT(*)").From(models.InvitationDBTable).Where(sq.Eq{models.InvitationDBIDColumn: returnedInvitation.ID}))
		require.NoError(t, err)
		require.Equal(t, 0, count)
	})
}
