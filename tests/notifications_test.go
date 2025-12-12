package tests

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/models"
)

func TestNotificationsFiltering(t *testing.T) {
	log := testutils.GetLogger(t)
	apiTesterConfigUpdater := NewAPITesterConfigUpdater(t, &log, true)
	tester := NewAPITester(t, apiTesterConfigUpdater.UpdateConfig)
	defer tester.Close()

	// Load fixtures:
	// - sessions.json: gondorID (public), shireID (public), isengardID (private)
	// - gandalf.json: global manager, member of gondor and shire
	// - merry_nosession.json: regular user, no session memberships
	// - gondor_members.json: boromir is member of gondor
	tester.LoadFixtureFile("fixtures/sessions.json")
	tester.LoadFixtureFile("fixtures/gandalf.json")
	tester.LoadFixtureFile("fixtures/merry_nosession.json")
	tester.LoadFixtureFile("fixtures/gondor_members.json")

	// Start the test server
	server := tester.TestServer()
	defer server.Close()

	// Helper to get WebSocket URL from HTTP URL
	wsURL := func(httpURL string) string {
		return "ws" + httpURL[4:] + "/api/v1/notifications"
	}

	// Helper to authenticate and get token
	getToken := func(nickname, apikey string) string {
		var token string
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/auth/authenticate", JSONObj{
			"nickname": nickname,
			"apikey":   apikey,
		}, &token))
		return token
	}

	// Helper to connect WebSocket with auth
	connectWS := func(token string) *websocket.Conn {
		header := http.Header{}
		header.Set("Authorization", "Bearer "+token)
		conn, resp, err := websocket.DefaultDialer.Dial(wsURL(server.URL), header)
		require.NoError(t, err)
		require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
		return conn
	}

	// Helper to read notification with timeout
	readNotification := func(conn *websocket.Conn, timeout time.Duration) (*notify.ResourceChange, error) {
		conn.SetReadDeadline(time.Now().Add(timeout))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return nil, err
		}
		var change notify.ResourceChange
		if err := jsoniter.Unmarshal(msg, &change); err != nil {
			return nil, err
		}
		return &change, nil
	}

	t.Run("invitation_notification_only_sent_to_invitee", func(t *testing.T) {
		// Get tokens for both users
		gandalfToken := getToken("GandalfTheGrey", "apikeyGandalf")
		merryToken := getToken("Merry", "apikeyMerry")
		boromirToken := getToken("BoromirDúnedain", "apikeyBoromir")

		// Connect all three users to WebSocket
		gandalfConn := connectWS(gandalfToken)
		defer gandalfConn.Close()

		merryConn := connectWS(merryToken)
		defer merryConn.Close()

		boromirConn := connectWS(boromirToken)
		defer boromirConn.Close()

		// Use mutex to safely collect notifications
		var mu sync.Mutex
		gandalfNotifications := make([]*notify.ResourceChange, 0)
		merryNotifications := make([]*notify.ResourceChange, 0)
		boromirNotifications := make([]*notify.ResourceChange, 0)

		// Start goroutines to listen for notifications
		var wg sync.WaitGroup

		collectNotifications := func(conn *websocket.Conn, notifications *[]*notify.ResourceChange, name string) {
			defer wg.Done()
			for {
				change, err := readNotification(conn, 2*time.Second)
				if err != nil {
					// Timeout or connection closed - expected
					return
				}
				mu.Lock()
				*notifications = append(*notifications, change)
				t.Logf("%s received notification: type=%s, id=%s, action=%s", name, change.Type, change.ID, change.Action)
				mu.Unlock()
			}
		}

		wg.Add(3)
		go collectNotifications(gandalfConn, &gandalfNotifications, "gandalf")
		go collectNotifications(merryConn, &merryNotifications, "merry")
		go collectNotifications(boromirConn, &boromirNotifications, "boromir")

		// Small delay to ensure WebSocket connections are established and subscriptions are active
		time.Sleep(100 * time.Millisecond)

		// Gandalf (global manager) invites Merry to gondor session
		tester.SetHeader("Authorization", "Bearer "+gandalfToken)
		var invite models.Invitation
		require.Equal(t, http.StatusCreated, tester.MustPostJSON("/api/v1/sessions/gondorID/invite", models.Invitation{
			SessionID:     "gondorID",
			UserProfileID: "merryID",
		}, &invite))

		// Wait for notifications to be collected
		wg.Wait()

		// Verify results:
		// - Gandalf (global manager) should see the invitation notification
		// - Merry (invitee) should see the invitation notification
		// - Boromir (not invitee, just gondor member) should NOT see the invitation notification

		mu.Lock()
		defer mu.Unlock()

		// Find invitation notifications
		findInvitationNotification := func(notifications []*notify.ResourceChange) *notify.ResourceChange {
			for _, n := range notifications {
				if n.Type == notify.TypeInvitation && n.ID == invite.ID {
					return n
				}
			}
			return nil
		}

		gandalfInviteNotif := findInvitationNotification(gandalfNotifications)
		merryInviteNotif := findInvitationNotification(merryNotifications)
		boromirInviteNotif := findInvitationNotification(boromirNotifications)

		// Gandalf is global manager, sees everything
		require.NotNil(t, gandalfInviteNotif, "gandalf (global manager) should receive invitation notification")
		require.Equal(t, notify.ActionCreated, gandalfInviteNotif.Action)

		// Merry is the invitee, should see the notification
		require.NotNil(t, merryInviteNotif, "merry (invitee) should receive invitation notification")
		require.Equal(t, notify.ActionCreated, merryInviteNotif.Action)

		// Boromir is NOT the invitee, should NOT see the invitation notification
		require.Nil(t, boromirInviteNotif, "boromir (not invitee) should NOT receive invitation notification")
	})

	t.Run("session_notification_filtered_by_membership_and_visibility", func(t *testing.T) {
		// Get tokens
		gandalfToken := getToken("GandalfTheGrey", "apikeyGandalf")
		merryToken := getToken("Merry", "apikeyMerry")
		boromirToken := getToken("BoromirDúnedain", "apikeyBoromir")

		// Connect all users to WebSocket
		gandalfConn := connectWS(gandalfToken)
		defer gandalfConn.Close()

		merryConn := connectWS(merryToken)
		defer merryConn.Close()

		boromirConn := connectWS(boromirToken)
		defer boromirConn.Close()

		var mu sync.Mutex
		gandalfNotifications := make([]*notify.ResourceChange, 0)
		merryNotifications := make([]*notify.ResourceChange, 0)
		boromirNotifications := make([]*notify.ResourceChange, 0)

		var wg sync.WaitGroup

		collectNotifications := func(conn *websocket.Conn, notifications *[]*notify.ResourceChange, name string) {
			defer wg.Done()
			for {
				change, err := readNotification(conn, 2*time.Second)
				if err != nil {
					return
				}
				mu.Lock()
				*notifications = append(*notifications, change)
				t.Logf("%s received notification: type=%s, id=%s, action=%s", name, change.Type, change.ID, change.Action)
				mu.Unlock()
			}
		}

		wg.Add(3)
		go collectNotifications(gandalfConn, &gandalfNotifications, "gandalf")
		go collectNotifications(merryConn, &merryNotifications, "merry")
		go collectNotifications(boromirConn, &boromirNotifications, "boromir")

		// Small delay to ensure WebSocket connections are established and subscriptions are active
		time.Sleep(100 * time.Millisecond)

		// Gandalf creates rules for gondor session (this triggers a session update notification)
		// Note: rulesCreate returns 200 OK (not 201 Created) when successful
		tester.SetHeader("Authorization", "Bearer "+gandalfToken)
		var ruleset models.Ruleset
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/sessions/gondorID/rules", models.Ruleset{
			UniverseSize:     3,
			Density:          2,
			StartingDistance: 3,
			RandomSeed:       54321,
		}, &ruleset))

		wg.Wait()

		mu.Lock()
		defer mu.Unlock()

		// Find session update notifications for gondorID
		findSessionNotification := func(notifications []*notify.ResourceChange, sessionID string) *notify.ResourceChange {
			for _, n := range notifications {
				if n.Type == notify.TypeSession && n.ID == sessionID {
					return n
				}
			}
			return nil
		}

		// gondorID is public, so everyone should see the notification
		gandalfSessionNotif := findSessionNotification(gandalfNotifications, "gondorID")
		merrySessionNotif := findSessionNotification(merryNotifications, "gondorID")
		boromirSessionNotif := findSessionNotification(boromirNotifications, "gondorID")

		require.NotNil(t, gandalfSessionNotif, "gandalf should receive gondorID session notification (global manager)")
		require.NotNil(t, merrySessionNotif, "merry should receive gondorID session notification (public session)")
		require.NotNil(t, boromirSessionNotif, "boromir should receive gondorID session notification (member)")
	})
}
