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

func TestPlayerOrderUpdateHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()
	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)
	// load some sessions because our players are members of them
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum.json")

	sessionReadHandler := NewSessionReadHandler(&log, testdb.DB)
	reorderHandler := NewReorderPlayersHandler(&log, testdb.DB)

	t.Run("merry_reorders_players_in_his_session_vs_gollum", func(t *testing.T) {
		merryVSGollumSessionID := "merryvsgollumID"
		merryID := "merryID"
		gollumID := "gollumID"

		merryPrincipal := models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   merryID,
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		sessionReadParams := operations.SessionReadParams{
			SessionID: merryVSGollumSessionID,
		}
		originalSession, err := sessionReadHandler.handle(ctx, sessionReadParams, &merryPrincipal)
		require.NoError(t, err)
		require.Equal(t, 2, len(originalSession.Players))
		require.Equal(t, merryID, originalSession.Players[0].UserProfileID)
		require.Equal(t, gollumID, originalSession.Players[1].UserProfileID)

		reorderParams := operations.ReorderPlayersParams{
			SessionID: merryVSGollumSessionID,
			Players: []*models.PlayerOrder{
				{
					PlayerOrder:   1,
					UserProfileID: merryID,
				},
				{
					PlayerOrder:   0,
					UserProfileID: gollumID,
				},
			},
		}
		playerOrders, err := reorderHandler.handle(ctx, reorderParams, &merryPrincipal)
		require.NoError(t, err)
		require.Equal(t, 2, len(playerOrders))

		reorderedSession, err := sessionReadHandler.handle(ctx, sessionReadParams, &merryPrincipal)
		require.NoError(t, err)
		require.Equal(t, 2, len(reorderedSession.Players))
		require.Equal(t, gollumID, reorderedSession.Players[0].UserProfileID) // <-- players have been re-ordered !!
		require.Equal(t, merryID, reorderedSession.Players[1].UserProfileID)
	})
}
