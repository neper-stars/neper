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
	"github.com/neper-stars/neper/lib/sessionSubmitter"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/migration"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/restapi/operations"
	"github.com/neper-stars/neper/sync"
)

func TestSessionPlayerSwitchToAIHandler(t *testing.T) {
	log := testutils.GetLogger(t)
	ctx := log.WithContext(context.Background())
	testdb := database.GetTestDB(ctx, t, migration.Source)
	defer testdb.Close()

	syncWorker, err := sync.NewWorker(testdb.DB, log)
	require.NoError(t, err)

	// Load a started session with turn files
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_merry.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_gollum.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_session_player_race.json")
	fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

	testSubmitter := sessionSubmitter.NewTestSessionSubmitter(&log)
	handler := NewSessionPlayerSwitchToAIHandler(&log, testdb.DB, testSubmitter, nil)

	t.Run("session_manager_can_switch_player_to_ai", func(t *testing.T) {
		// Merry is the session manager
		merryPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// First, verify initial state - both players should be human
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var sessionFilesDB models.SessionFilesDB
		require.NoError(t, sqlH.GetBy(&sessionFilesDB, "session_id", "merryvsgollumID"))

		hstData, err := stars.B64Decode(sessionFilesDB.HostFile)
		require.NoError(t, err)

		fileInfo, err := playerchanger.ReadPlayersFromBytes("game.hst", hstData)
		require.NoError(t, err)
		t.Logf("Initial players: %d", len(fileInfo.Players))
		for _, p := range fileInfo.Players {
			t.Logf("  Player %d: %s, Status: %s (%d)", p.Number, p.Name, p.Status, p.StatusType)
		}

		// Switch player 1 (gollum) to AI (CA - Claim Adjuster)
		params := operations.SessionPlayerSwitchToAIParams{
			SessionID:   "merryvsgollumID",
			PlayerOrder: 1,
			SwitchRequest: &models.SwitchToAiRequest{
				AiType: "CA",
			},
		}

		err = handler.handle(ctx, params, merryPrincipal)
		require.NoError(t, err)

		// Verify the player is now AI by reading the host file from database
		var sessionFilesDBAfter models.SessionFilesDB
		require.NoError(t, sqlH.GetBy(&sessionFilesDBAfter, "session_id", "merryvsgollumID"))

		// Decode and parse the host file
		hstDataAfter, err := stars.B64Decode(sessionFilesDBAfter.HostFile)
		require.NoError(t, err)

		fileInfoAfter, err := playerchanger.ReadPlayersFromBytes("game.hst", hstDataAfter)
		require.NoError(t, err)

		t.Logf("After switch players:")
		for _, p := range fileInfoAfter.Players {
			t.Logf("  Player %d: %s, Status: %s (%d)", p.Number, p.Name, p.Status, p.StatusType)
		}

		// Get player 1's info
		player1 := fileInfoAfter.GetPlayer(1)
		require.NotNil(t, player1, "player 1 should exist in the host file")

		// Verify player is now AI
		require.Equal(t, store.PlayerStatusAI, player1.StatusType, "player 1 should be AI")
		require.Equal(t, store.AIExpertCA, player1.AIExpertType, "player 1 should be CA AI")
	})

	t.Run("global_manager_can_switch_player_to_ai", func(t *testing.T) {
		// Reset by reloading fixtures
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_merry.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_gollum.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_session_player_race.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

		// A global manager who is not a member of the session
		globalManagerPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "someOtherUserID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		// Switch player 0 (merry) to AI (HE - Hyper Expansion)
		params := operations.SessionPlayerSwitchToAIParams{
			SessionID:   "merryvsgollumID",
			PlayerOrder: 0,
			SwitchRequest: &models.SwitchToAiRequest{
				AiType: "HE",
			},
		}

		err := handler.handle(ctx, params, globalManagerPrincipal)
		require.NoError(t, err)

		// Verify the player is now AI
		sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
		var sessionFilesDB models.SessionFilesDB
		require.NoError(t, sqlH.GetBy(&sessionFilesDB, "session_id", "merryvsgollumID"))

		hstData, err := stars.B64Decode(sessionFilesDB.HostFile)
		require.NoError(t, err)

		fileInfo, err := playerchanger.ReadPlayersFromBytes("game.hst", hstData)
		require.NoError(t, err)

		player0 := fileInfo.GetPlayer(0)
		require.NotNil(t, player0, "player 0 should exist in the host file")
		require.Equal(t, store.PlayerStatusAI, player0.StatusType, "player 0 should be AI")
		require.Equal(t, store.AIExpertHE, player0.AIExpertType, "player 0 should be HE AI")
	})

	t.Run("all_ai_types_work_correctly", func(t *testing.T) {
		aiTypes := []struct {
			code     string
			expected store.AIExpertType
		}{
			{"HE", store.AIExpertHE},
			{"SS", store.AIExpertSS},
			{"IS", store.AIExpertIS},
			{"CA", store.AIExpertCA},
			{"PP", store.AIExpertPP},
			{"AR", store.AIExpertAR},
		}

		for _, aiType := range aiTypes {
			t.Run("ai_type_"+aiType.code, func(t *testing.T) {
				// Reset by reloading fixtures
				fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
				fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

				merryPrincipal := &models.Principal{
					StandardClaims: jwt.StandardClaims{
						Subject:   "merryID",
						ExpiresAt: time.Now().Add(time.Minute).Unix(),
					},
					IsGlobalManager: false,
				}

				params := operations.SessionPlayerSwitchToAIParams{
					SessionID:   "merryvsgollumID",
					PlayerOrder: 1,
					SwitchRequest: &models.SwitchToAiRequest{
						AiType: aiType.code,
					},
				}

				err := handler.handle(ctx, params, merryPrincipal)
				require.NoError(t, err)

				// Verify
				sqlH := database.NewSQLHelper(ctx, testdb.DB, log)
				var sessionFilesDB models.SessionFilesDB
				require.NoError(t, sqlH.GetBy(&sessionFilesDB, "session_id", "merryvsgollumID"))

				hstData, err := stars.B64Decode(sessionFilesDB.HostFile)
				require.NoError(t, err)

				fileInfo, err := playerchanger.ReadPlayersFromBytes("game.hst", hstData)
				require.NoError(t, err)

				player1 := fileInfo.GetPlayer(1)
				require.NotNil(t, player1)
				require.Equal(t, store.PlayerStatusAI, player1.StatusType, "player should be AI")
				require.Equal(t, aiType.expected, player1.AIExpertType, "AI type should be "+aiType.code)
			})
		}
	})

	t.Run("regular_member_cannot_switch_player", func(t *testing.T) {
		// Reset fixtures
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

		// Gollum is a regular member, not a manager
		gollumPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "gollumID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		params := operations.SessionPlayerSwitchToAIParams{
			SessionID:   "merryvsgollumID",
			PlayerOrder: 0,
			SwitchRequest: &models.SwitchToAiRequest{
				AiType: "CA",
			},
		}

		err := handler.handle(ctx, params, gollumPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden), "regular member should get forbidden error")
	})

	t.Run("non_member_cannot_switch_player", func(t *testing.T) {
		// Reset fixtures
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

		// Someone who is not a member at all
		nonMemberPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "randomUserID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		params := operations.SessionPlayerSwitchToAIParams{
			SessionID:   "merryvsgollumID",
			PlayerOrder: 0,
			SwitchRequest: &models.SwitchToAiRequest{
				AiType: "CA",
			},
		}

		err := handler.handle(ctx, params, nonMemberPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrForbidden), "non-member should get forbidden error")
	})

	t.Run("precondition_failed_when_game_not_started", func(t *testing.T) {
		// Load a different session without turn files (game not started)
		// Using a session that doesn't have any session_files loaded
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/sessions.json")

		// Use a global manager to bypass session membership check for this test
		globalManagerPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "someGlobalManagerID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: true,
		}

		params := operations.SessionPlayerSwitchToAIParams{
			SessionID:   "gondorID", // This session has no session files
			PlayerOrder: 0,
			SwitchRequest: &models.SwitchToAiRequest{
				AiType: "CA",
			},
		}

		err := handler.handle(ctx, params, globalManagerPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrPreconditionFailed), "should get precondition failed when game not started")
	})

	t.Run("cannot_switch_last_human_player", func(t *testing.T) {
		// Reset fixtures
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

		// Merry is the session manager
		merryPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// First, switch player 1 to AI (this should work)
		params := operations.SessionPlayerSwitchToAIParams{
			SessionID:   "merryvsgollumID",
			PlayerOrder: 1,
			SwitchRequest: &models.SwitchToAiRequest{
				AiType: "CA",
			},
		}
		err := handler.handle(ctx, params, merryPrincipal)
		require.NoError(t, err, "switching first player to AI should work")

		// Now try to switch player 0 to AI - this should fail as it's the last human
		params2 := operations.SessionPlayerSwitchToAIParams{
			SessionID:   "merryvsgollumID",
			PlayerOrder: 0,
			SwitchRequest: &models.SwitchToAiRequest{
				AiType: "HE",
			},
		}
		err = handler.handle(ctx, params2, merryPrincipal)
		require.Error(t, err)
		require.True(t, errors.Is(err, errs.ErrPreconditionFailed), "should get precondition failed when trying to switch last human player")
		require.Contains(t, err.Error(), "last human", "error should mention last human player")
	})

	t.Run("triggers_turn_generation_when_last_pending_human_switched_to_ai", func(t *testing.T) {
		// Reset fixtures - fixture has player 0 (merry) submitted, player 1 (gollum) not submitted
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_merry.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_gollum.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_session_player_race.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

		// Clear any previous messages from the test submitter
		testSubmitter.Msgs = nil

		merryPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Switch player 1 (gollum) to AI - player 1 has NOT submitted orders
		// Since player 0 (merry) has already submitted, this should trigger turn generation
		params := operations.SessionPlayerSwitchToAIParams{
			SessionID:   "merryvsgollumID",
			PlayerOrder: 1,
			SwitchRequest: &models.SwitchToAiRequest{
				AiType: "CA",
			},
		}

		err := handler.handle(ctx, params, merryPrincipal)
		require.NoError(t, err)

		// Verify turn generation was triggered
		require.Len(t, testSubmitter.Msgs, 1, "turn generation should be triggered when last pending human is switched to AI")
		require.Equal(t, "Turn.NeedsGeneration", testSubmitter.Msgs[0].Subject)
	})

	t.Run("does_not_trigger_turn_generation_when_other_humans_still_pending", func(t *testing.T) {
		// Reset fixtures - fixture has player 0 (merry) submitted, player 1 (gollum) not submitted
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_started.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_merry.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/race_gollum.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_session_player_race.json")
		fixtures.LoadFixtureFile(t, syncWorker, "fixtures/merryvsgollum_turn0_files.json")

		// Clear any previous messages from the test submitter
		testSubmitter.Msgs = nil

		merryPrincipal := &models.Principal{
			StandardClaims: jwt.StandardClaims{
				Subject:   "merryID",
				ExpiresAt: time.Now().Add(time.Minute).Unix(),
			},
			IsGlobalManager: false,
		}

		// Switch player 0 (merry) to AI - player 0 has already submitted
		// Since player 1 (gollum) has NOT submitted, turn generation should NOT be triggered
		params := operations.SessionPlayerSwitchToAIParams{
			SessionID:   "merryvsgollumID",
			PlayerOrder: 0,
			SwitchRequest: &models.SwitchToAiRequest{
				AiType: "HE",
			},
		}

		err := handler.handle(ctx, params, merryPrincipal)
		require.NoError(t, err)

		// Verify turn generation was NOT triggered because player 1 still hasn't submitted
		require.Len(t, testSubmitter.Msgs, 0, "turn generation should NOT be triggered when other humans still haven't submitted")
	})
}
