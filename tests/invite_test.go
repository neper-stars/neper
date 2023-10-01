package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/neper-stars/neper/models"
)

func TestInvite(t *testing.T) {
	tester := NewAPITester(t, nil)
	defer tester.Close()

	tester.LoadFixtureFile("fixtures/sessions.json")
	tester.LoadFixtureFile("fixtures/gandalf.json")
	tester.LoadFixtureFile("fixtures/merry_nosession.json")
	tester.LoadFixtureFile("fixtures/gondor_members.json")

	tester.Run("invite_authorized", func(t *testing.T) { //nolint:thelper
		defer tester.SetHeader("Authorization", "")

		var token string
		var invite models.Invitation

		boromirNickName := "BoromirDúnedain"
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/auth/authenticate", JSONObj{
			"nickname": boromirNickName,
			"apikey":   "apikeyBoromir",
		}, &token))

		tester.SetHeader("Authorization", "Bearer "+token)

		// boromir cannot invite in the shire... he is not manager of the shire
		require.Equal(t, http.StatusForbidden, tester.MustPostJSON("/api/v1/sessions/shireID/invite", models.Invitation{
			SessionID:     "shireID",
			UserProfileID: "merryID",
		}, &invite))

		gandalfNickName := "GandalfTheGrey"
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/auth/authenticate", JSONObj{
			"nickname": gandalfNickName,
			"apikey":   "apikeyGandalf",
		}, &token))

		tester.SetHeader("Authorization", "Bearer "+token)

		// gandalf invites merry to the shire
		require.Equal(t, http.StatusCreated, tester.MustPostJSON("/api/v1/sessions/shireID/invite", models.Invitation{
			SessionID:     "shireID",
			UserProfileID: "merryID",
		}, &invite))

	})
}
