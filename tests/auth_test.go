package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neper-stars/neper/models"
	"orus.io/orus-io/go-orusapi/testutils"
)

func TestAuthentication(t *testing.T) {
	log := testutils.GetLogger(t)
	apiTesterConfigUpdater := NewAPITesterConfigUpdater(t, &log)
	tester := NewAPITester(t, apiTesterConfigUpdater.UpdateConfig)
	defer tester.Close()
	/*
		log := testutils.GetLogger(t)
		runner, shutdown := stars.GetTestStarsRunner(t, &log)
		defer shutdown()
		tester.Config.StarsRunner = runner
	*/

	tester.LoadFixtureFile("fixtures/sessions.json")
	tester.LoadFixtureFile("fixtures/gandalf.json")
	tester.LoadFixtureFile("fixtures/merry_nosession.json")
	tester.LoadFixtureFile("fixtures/gondor_members.json")

	tester.Run("WebToken", func(t *testing.T) { //nolint:thelper
		defer tester.SetHeader("Authorization", "")

		var token string
		gandalfNickName := "GandalfTheGrey"
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/auth/authenticate", JSONObj{
			"nickname": gandalfNickName,
			"apikey":   "apikeyGandalf",
		}, &token))

		tester.SetHeader("Authorization", "Bearer "+token)

		var userinfo models.Userinfo
		require.Equal(t, http.StatusOK, tester.MustGetJSON("/api/v1/auth/userinfo", &userinfo))
		require.Equal(t, gandalfNickName, userinfo.User.Nickname)
	})
}
