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

func TestTurnSubmit(t *testing.T) {
	log := testutils.GetLogger(t)
	autoDelete := false
	apiTesterConfigUpdater := NewAPITesterConfigUpdater(t, &log, autoDelete)
	tester := NewAPITester(t, apiTesterConfigUpdater.UpdateConfig)
	defer tester.Close()

	tester.LoadFixtureFile("../restapi/handlers/fixtures/merryvsgollum.json")
	tester.LoadFixtureFile("../restapi/handlers/fixtures/merryvsgollum_turn0_files.json")

	// reset Auth after test
	defer tester.SetHeader("Authorization", "")

	var token string

	t.Run("gollum_submits_his_turn_and_receives_notification", func(t *testing.T) {
		gollumNickName := "gollum"
		apiKeyGollum := "apikeyGollum"
		sessionID := "merryvsgollumID"
		year := 2400
		gollumOrderFileName := "../restapi/handlers/fixtures/merryvsgollum/Game.x2"
		gf, err := os.Open(gollumOrderFileName)
		require.NoError(t, err)
		gollumOrderContent, err := io.ReadAll(gf)
		require.NoError(t, err)
		gollumOrderContentB64 := stars.B64Encode(gollumOrderContent)

		// login as gollum
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/auth/authenticate", JSONObj{
			"nickname": gollumNickName,
			"apikey":   apiKeyGollum,
		}, &token))
		tester.SetHeader("Authorization", "Bearer "+token)

		// Connect to notifications WebSocket first
		d := wstest.NewDialer(tester.handler)
		header := make(http.Header)
		header.Set("Authorization", "Bearer "+token)

		c, resp, err := d.Dial("ws://whatever/api/v1/notifications", header)
		require.NoError(t, err)
		require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
		defer func() {
			if err := c.Close(); err != nil {
				t.Logf("failed to close websocket connection: %v", err)
			}
		}()

		// Set a read deadline so we don't wait forever
		require.NoError(t, c.SetReadDeadline(time.Now().Add(30*time.Second)))

		// Submit the turn
		submitURL := fmt.Sprintf("/api/v1/sessions/%s/turn/%d", sessionID, year)
		turn := types.Order{B64Data: gollumOrderContentB64}
		require.Equal(t, http.StatusOK, tester.MustPutJSON(submitURL, &turn, &token))

		// Helper to safely dereference string pointer
		ptrStr := func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		}

		// Wait for session_turn notification
		var notification notify.ResourceChange
		for {
			_, message, err := c.ReadMessage()
			require.NoError(t, err)

			err = json.Unmarshal(message, &notification)
			require.NoError(t, err)

			// Look for session_turn notification for our session
			if ptrStr(notification.Type) == notify.ResourceChangeTypeSessionTurn &&
				ptrStr(notification.ID) == sessionID &&
				ptrStr(notification.Action) == notify.ResourceChangeActionReady {
				break
			}
			// Continue waiting for more notifications
		}

		// Verify the notification contains the correct year in metadata
		meta := notify.ParseMetadata[notify.SessionTurnMeta](&notification)
		require.NotNil(t, meta, "year should be present in metadata")
		require.Equal(t, int64(year+1), meta.Year)

		// Now fetch the turn data via regular HTTP GET
		getURL := fmt.Sprintf("/api/v1/sessions/%s/turn/%d", sessionID, year+1)
		var turnFiles models.TurnFiles
		require.Equal(t, http.StatusOK, tester.MustGetJSON(getURL, &turnFiles))

		// Verify the turn data
		require.Equal(t, int64(year+1), turnFiles.Year)
	})
}
