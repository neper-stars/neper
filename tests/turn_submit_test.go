package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"fmt"

	"io"
	"os"

	"github.com/neper-stars/neper/lib/stars"
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

		// submit turn
		// but first ask for a connection upgrade to websocket in order
		// to receive the turn when it is ready
		// tester.SetHeader("Connection", "upgrade")
		// tester.SetHeader("Upgrade", "websocket")

		submitURL := fmt.Sprintf("/api/v1/sessions/%s/turn/%d", sessionID, year)
		turn := types.Order{B64Data: gollumOrderContentB64}
		require.Equal(t, http.StatusOK, tester.MustPutJSON(submitURL, &turn, &token))
	})
}
