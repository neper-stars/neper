package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neper-stars/neper/models"
	"orus.io/orus-io/go-orusapi/testutils"
)

func TestInvite(t *testing.T) {
	log := testutils.GetLogger(t)
	apiTesterConfigUpdater := NewAPITesterConfigUpdater(t, &log, true)
	tester := NewAPITester(t, apiTesterConfigUpdater.UpdateConfig)
	defer tester.Close()

	tester.LoadFixtureFile("fixtures/sessions.json")
	tester.LoadFixtureFile("fixtures/gandalf.json")
	tester.LoadFixtureFile("fixtures/shire_members.json")
	tester.LoadFixtureFile("fixtures/merry_nosession.json")
	tester.LoadFixtureFile("fixtures/sam_nosession.json")
	tester.LoadFixtureFile("fixtures/gondor_members.json")

	defer tester.SetHeader("Authorization", "")

	var token string
	var invite models.Invitation

	t.Run("boromir_cannot_invite_to_shire", func(t *testing.T) {
		boromirNickName := "BoromirDúnedain"
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/auth/authenticate", JSONObj{
			"nickname": boromirNickName,
			"apikey":   "apikeyBoromir",
		}, &token))

		tester.SetHeader("Authorization", "Bearer "+token)

		// Boromir cannot invite in the shire... he is not manager of the shire
		require.Equal(t, http.StatusForbidden, tester.MustPostJSON("/api/v1/sessions/shireID/invite", models.Invitation{
			SessionID:     "shireID",
			UserProfileID: "merryID",
		}, &invite))
	})

	t.Run("frodo_can_invite_to_shire", func(t *testing.T) {
		// now try with Frodo who is the manager of the shire session (not general manager)
		frodoNickName := "frodo"
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/auth/authenticate", JSONObj{
			"nickname": frodoNickName,
			"apikey":   "apikeyFrodo",
		}, &token))

		tester.SetHeader("Authorization", "Bearer "+token)

		// Frodo invites merry to the shire
		require.Equal(t, http.StatusCreated, tester.MustPostJSON("/api/v1/sessions/shireID/invite", models.Invitation{
			SessionID:     "shireID",
			UserProfileID: "merryID",
		}, &invite))
	})

	t.Run("gandalf_can_invite_to_shire", func(t *testing.T) {
		// now try with Gandalf who is general manager
		gandalfNickName := "GandalfTheGrey"
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/auth/authenticate", JSONObj{
			"nickname": gandalfNickName,
			"apikey":   "apikeyGandalf",
		}, &token))

		tester.SetHeader("Authorization", "Bearer "+token)

		// gandalf invites sam to the shire
		require.Equal(t, http.StatusCreated, tester.MustPostJSON("/api/v1/sessions/shireID/invite", models.Invitation{
			SessionID:     "shireID",
			UserProfileID: "samID",
		}, &invite))
	})
}
