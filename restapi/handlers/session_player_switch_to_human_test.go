package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/neper-stars/houston/lib/tools/playerchanger"
	"github.com/neper-stars/houston/store"
	"github.com/stretchr/testify/require"
	"gopkg.in/dgrijalva/jwt-go.v3"
	"orus.io/orus-io/go-orusapi/database"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/fixtures"
	errs "github.com/neper-stars/neper/lib/errors"
	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
	"github.com/neper-stars/neper/sync"
)

func TestSessionPlayerSwitchToHumanHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	// Load fixtures
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_merry.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_gollum.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_session_player_race.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

	testNotify := notify.NewTestNotifyService(&log)
	handler := NewSessionPlayerSwitchToHumanHandler(&log, testdb.DB, testNotify)

	// Helper to switch player to AI first (to set up for switch back to human tests)
	switchToAI := func(t *testing.T, playerOrder int64, aiType string) {
		t.Helper()
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var sessionFilesDB models.SessionFilesDB
		require.NoError(t, sqlH.GetBy(&sessionFilesDB, "session_id", "merryvsgollumID"))

		hstData, err := stars.B64Decode(sessionFilesDB.HostFile)
		require.NoError(t, err)

		aiTypeCode, err := store.ParseAIExpertType(aiType)
		require.NoError(t, err)

		modifiedHst, _, err := playerchanger.ChangeToAIBytes(hstData, int(playerOrder), aiTypeCode)
		require.NoError(t, err)

		sessionFilesDB.HostFile = stars.B64Encode(modifiedHst)
		require.NoError(t, sqlH.UpdateColumns(&sessionFilesDB, models.SessionFilesDBHostFileColumn))

		// Also update session_player_race ai_control_type
		var spr models.SessionPlayerRaceDB
		query := sessionPlayerRaceByOrderQuery("merryvsgollumID", int(playerOrder))
		require.NoError(t, sqlH.Get(&spr, query))
		spr.AIControlType = &aiType
		require.NoError(t, sqlH.UpdateColumns(&spr, models.SessionPlayerRaceDBAIControlTypeColumn))
	}

	t.Run("session_manager_can_switch_player_to_human", func(t *testing.T) {
		// Reset fixtures
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_merry.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_gollum.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_session_player_race.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

		// First switch player 1 to AI
		switchToAI(t, 1, "CA")

		// Verify player is AI
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var sessionFilesDB models.SessionFilesDB
		require.NoError(t, sqlH.GetBy(&sessionFilesDB, "session_id", "merryvsgollumID"))
		hstData, err := stars.B64Decode(sessionFilesDB.HostFile)
		require.NoError(t, err)
		fileInfo, err := playerchanger.ReadPlayersFromBytes("game.hst", hstData)
		require.NoError(t, err)
		player1Before := fileInfo.GetPlayer(1)
		require.Equal(t, store.PlayerStatusAI, player1Before.StatusType, "player 1 should be AI before switch")

		// Merry is the session manager
		merryPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		testNotify.Clear()

		// Switch player 1 back to human
		params := operations.SessionPlayerSwitchToHumanParams{
			SessionID:   "merryvsgollumID",
			PlayerOrder: 1,
		}

		err = handler.handle(ctx, params, merryPrincipal)
		require.NoError(t, err)

		// Verify the player is now human
		var sessionFilesDBAfter models.SessionFilesDB
		require.NoError(t, sqlH.GetBy(&sessionFilesDBAfter, "session_id", "merryvsgollumID"))
		hstDataAfter, err := stars.B64Decode(sessionFilesDBAfter.HostFile)
		require.NoError(t, err)
		fileInfoAfter, err := playerchanger.ReadPlayersFromBytes("game.hst", hstDataAfter)
		require.NoError(t, err)

		player1After := fileInfoAfter.GetPlayer(1)
		require.NotNil(t, player1After, "player 1 should exist in the host file")
		require.Equal(t, store.PlayerStatusHuman, player1After.StatusType, "player 1 should be human")

		// Verify ai_control_type is NULL in database
		var sprAfter models.SessionPlayerRaceDB
		sprQuery := sessionPlayerRaceByOrderQuery("merryvsgollumID", 1)
		require.NoError(t, sqlH.Get(&sprAfter, sprQuery))
		require.Nil(t, sprAfter.AIControlType, "ai_control_type should be NULL after switch to human")
	})

	t.Run("global_manager_can_switch_player_to_human", func(t *testing.T) {
		// Reset fixtures
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_merry.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_gollum.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_session_player_race.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

		// First switch player 0 to AI
		switchToAI(t, 0, "HE")

		// A global manager who is not a member of the session
		globalManagerPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "someOtherUserID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		testNotify.Clear()

		// Switch player 0 back to human
		params := operations.SessionPlayerSwitchToHumanParams{
			SessionID:   "merryvsgollumID",
			PlayerOrder: 0,
		}

		err := handler.handle(ctx, params, globalManagerPrincipal)
		require.NoError(t, err)

		// Verify the player is now human
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var sessionFilesDB models.SessionFilesDB
		require.NoError(t, sqlH.GetBy(&sessionFilesDB, "session_id", "merryvsgollumID"))
		hstData, err := stars.B64Decode(sessionFilesDB.HostFile)
		require.NoError(t, err)
		fileInfo, err := playerchanger.ReadPlayersFromBytes("game.hst", hstData)
		require.NoError(t, err)

		player0 := fileInfo.GetPlayer(0)
		require.NotNil(t, player0, "player 0 should exist in the host file")
		require.Equal(t, store.PlayerStatusHuman, player0.StatusType, "player 0 should be human")
	})

	t.Run("regular_member_cannot_switch_player", func(t *testing.T) {
		// Reset fixtures
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_merry.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_gollum.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_session_player_race.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

		// First switch player 0 to AI
		switchToAI(t, 0, "CA")

		// Gollum is a regular member, not a manager
		gollumPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "gollumID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		params := operations.SessionPlayerSwitchToHumanParams{
			SessionID:   "merryvsgollumID",
			PlayerOrder: 0,
		}

		err := handler.handle(ctx, params, gollumPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden), "regular member should get forbidden error")
	})

	t.Run("non_member_cannot_switch_player", func(t *testing.T) {
		// Reset fixtures
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_merry.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_gollum.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_session_player_race.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

		// First switch player 0 to AI
		switchToAI(t, 0, "CA")

		// Someone who is not a member at all
		nonMemberPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "randomUserID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		params := operations.SessionPlayerSwitchToHumanParams{
			SessionID:   "merryvsgollumID",
			PlayerOrder: 0,
		}

		err := handler.handle(ctx, params, nonMemberPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden), "non-member should get forbidden error")
	})

	t.Run("precondition_failed_when_player_already_human", func(t *testing.T) {
		// Reset fixtures - players start as human
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_merry.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_gollum.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_session_player_race.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

		merryPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Try to switch player 1 to human when they're already human
		params := operations.SessionPlayerSwitchToHumanParams{
			SessionID:   "merryvsgollumID",
			PlayerOrder: 1,
		}

		err := handler.handle(ctx, params, merryPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrPreconditionFailed), "should get precondition failed when player is already human")
		require.Contains(t, err.Error(), "already human", "error should mention already human")
	})

	t.Run("precondition_failed_when_game_not_started", func(t *testing.T) {
		// Load a session without turn files (game not started)
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")

		// Use a global manager to bypass session membership check
		globalManagerPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "someGlobalManagerID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		params := operations.SessionPlayerSwitchToHumanParams{
			SessionID:   "gondorID", // This session has no session files
			PlayerOrder: 0,
		}

		err := handler.handle(ctx, params, globalManagerPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrPreconditionFailed), "should get precondition failed when game not started")
	})

	t.Run("sends_notification_when_switching_to_human", func(t *testing.T) {
		// Reset fixtures
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_merry.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_gollum.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_session_player_race.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

		// First switch player 1 to AI
		switchToAI(t, 1, "CA")

		merryPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		testNotify.Clear()

		// Switch player 1 back to human
		params := operations.SessionPlayerSwitchToHumanParams{
			SessionID:   "merryvsgollumID",
			PlayerOrder: 1,
		}

		err := handler.handle(ctx, params, merryPrincipal)
		require.NoError(t, err)

		// Verify notification was sent
		notifications := testNotify.GetPlayerControlMessages()
		require.Len(t, notifications, 1, "should have sent one player_control notification")

		notif := notifications[0]
		require.Equal(t, notify.ResourceChangeTypePlayerControl, *notif.Type)
		require.Equal(t, "merryvsgollumID", *notif.ID)
		require.Equal(t, notify.ResourceChangeActionUpdated, *notif.Action)

		// Parse metadata and verify
		meta := notify.ParseMetadata[notify.PlayerControlMeta](&notif)
		require.NotNil(t, meta, "notification should have metadata")
		require.Equal(t, "merryvsgollumID", meta.SessionID)
		require.Equal(t, int64(1), meta.PlayerOrder)
		require.Nil(t, meta.AIControlType, "ai_control_type should be nil when switching to human")
	})
}
