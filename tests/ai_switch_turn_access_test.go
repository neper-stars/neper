package tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/posener/wstest"
	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/models/types"
)

// TestAISwitchTurnAccess tests the scenario where:
// 1. A game is started with two human players
// 2. Player 0 submits their turn
// 3. Player 1 is switched to AI (which triggers turn generation since all remaining humans submitted)
// 4. Player 1 (now AI-controlled) tries to get their latest turn
//
// This test reproduces the bug where the Turns array indices don't match player orders
// because bots were being skipped during turn file collection, causing a panic when
// accessing Turns[playerOrder].
func TestAISwitchTurnAccess(t *testing.T) {
	log := testutils.GetLogger(t)
	autoDelete := false
	apiTesterConfigUpdater := NewAPITesterConfigUpdater(t, &log, autoDelete)
	tester := NewAPITester(t, apiTesterConfigUpdater.UpdateConfig)
	defer tester.Close()

	// Load fixtures for a started game with two human players (no orders submitted yet)
	tester.LoadFixtureFile("../restapi/handlers/fixtures/merryvsgollum.json")
	tester.LoadFixtureFile("fixtures/merryvsgollum_turn0_no_orders.json")

	// Reset Auth after test
	defer tester.SetHeader("Authorization", "")

	sessionID := "merryvsgollumID"
	year := 2400

	// Read order files for both players
	merryOrderFileName := "../restapi/handlers/fixtures/merryvsgollum/Game.x1"
	mf, err := os.Open(merryOrderFileName)
	require.NoError(t, err)
	merryOrderContent, err := io.ReadAll(mf)
	require.NoError(t, err)
	merryOrderContentB64 := stars.B64Encode(merryOrderContent)

	t.Run("ai_controlled_player_can_get_latest_turn", func(t *testing.T) {
		var merryToken string
		var gollumToken string

		// Login as merry (player 0, session manager)
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/auth/authenticate", JSONObj{
			"nickname": "merry",
			"apikey":   "apikeyMerry",
		}, &merryToken))

		// Login as gollum (player 1)
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/auth/authenticate", JSONObj{
			"nickname": "gollum",
			"apikey":   "apikeyGollum",
		}, &gollumToken))

		// Connect to notifications WebSocket as merry to receive turn ready notification
		tester.SetHeader("Authorization", "Bearer "+merryToken)
		d := wstest.NewDialer(tester.handler)
		header := make(http.Header)
		header.Set("Authorization", "Bearer "+merryToken)

		c, resp, err := d.Dial("ws://whatever/api/v1/notifications", header)
		require.NoError(t, err)
		require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
		defer func() {
			if err := c.Close(); err != nil {
				t.Logf("failed to close websocket connection: %v", err)
			}
		}()

		// Set a read deadline
		require.NoError(t, c.SetReadDeadline(time.Now().Add(60*time.Second)))

		// Step 1: Merry (player 0) submits their turn
		submitURL := fmt.Sprintf("/api/v1/sessions/%s/turn/%d", sessionID, year)
		turn := types.Order{B64Data: merryOrderContentB64}
		require.Equal(t, http.StatusOK, tester.MustPutJSON(submitURL, &turn, &merryToken))

		// Step 2: Switch Gollum (player 1) to AI
		// This should trigger turn generation since the only remaining human (merry) has submitted
		switchToAIURL := fmt.Sprintf("/api/v1/sessions/%s/player/%d/switch_to_ai", sessionID, 1)
		switchRequest := JSONObj{
			"ai_type": "CA", // Computer Average
		}
		require.Equal(t, http.StatusNoContent, tester.MustPostJSON(switchToAIURL, switchRequest, nil))

		// Helper to safely dereference string pointer
		ptrStr := func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		}

		// Wait for session_turn notification indicating turn generation completed
		var notification notify.ResourceChange
		for {
			_, message, err := c.ReadMessage()
			require.NoError(t, err)

			err = json.Unmarshal(message, &notification)
			require.NoError(t, err)

			t.Logf("Received notification: type=%s id=%s action=%s",
				ptrStr(notification.Type), ptrStr(notification.ID), ptrStr(notification.Action))

			// Look for session_turn notification for our session
			if ptrStr(notification.Type) == notify.ResourceChangeTypeSessionTurn &&
				ptrStr(notification.ID) == sessionID &&
				ptrStr(notification.Action) == notify.ResourceChangeActionReady {
				break
			}
		}

		// Verify the new turn year
		meta := notify.ParseMetadata[notify.SessionTurnMeta](&notification)
		require.NotNil(t, meta, "year should be present in metadata")
		require.Equal(t, int64(year+1), meta.Year)

		// Step 3: Gollum (AI-controlled player) tries to get their latest turn
		// This is the key test - it should NOT panic even though gollum is AI-controlled
		tester.SetHeader("Authorization", "Bearer "+gollumToken)
		getLatestURL := fmt.Sprintf("/api/v1/sessions/%s/turn/latest", sessionID)
		var turnFiles models.TurnFiles
		status := tester.MustGetJSON(getLatestURL, &turnFiles)

		// The request should succeed (not panic with index out of range)
		require.Equal(t, http.StatusOK, status, "AI-controlled player should be able to get their latest turn")
		require.Equal(t, int64(year+1), turnFiles.Year)
		require.NotNil(t, turnFiles.Turn, "turn data should not be nil")
		require.NotEmpty(t, turnFiles.Turn.Turn, "turn file should not be empty")
	})
}
