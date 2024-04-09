package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"fmt"

	"io"
	"os"

	"github.com/posener/wstest"

	"time"

	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/models/types"
	"orus.io/orus-io/go-orusapi/testutils"
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
	// reset connection upgrade request
	defer tester.SetHeader("Connection", "")
	defer tester.SetHeader("Upgrade", "")

	var token string

	t.Run("gollum_submits_his_turn_and_waits_for_a_new_turn", func(t *testing.T) {
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

		submitURL := fmt.Sprintf("/api/v1/sessions/%s/turn/%d", sessionID, year)
		turn := types.Order{B64Data: gollumOrderContentB64}
		require.Equal(t, http.StatusOK, tester.MustPutJSON(submitURL, &turn, &token))

		d := wstest.NewDialer(tester.handler)

		getURL := fmt.Sprintf("/api/v1/sessions/%s/turn/%d", sessionID, year+1)
		// var newTurn models.TurnFiles

		// tester.SetHeader("Connection", "upgrade")
		// tester.SetHeader("Upgrade", "websocket")
		// tester.SetHeader("Sec-WebSocket-Version", "13")
		// uid, err := uuid.V4()
		// require.NoError(t, err)
		// key := uid.String()
		// tester.SetHeader("Sec-WebSocket-Key", key)

		header := make(http.Header)
		// authorize our dialer
		header.Set("Authorization", "Bearer "+token)

		c, resp, err := d.Dial("ws://"+"whatever"+getURL, header)
		// tester.MustGetJSON(getURL, &newTurn)
		// require.Equal(t, int64(2401), newTurn.Year)
		require.NoError(t, err)
		require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

		// here we should wait for our new turn
		var tf models.TurnFiles
		var ti time.Time
		require.NoError(t, c.SetReadDeadline(ti))
		err = c.ReadJSON(&tf)
		require.NoError(t, err)

		// returned turn should be for year 2401
		require.Equal(t, int64(2401), tf.Year)
	})
}
