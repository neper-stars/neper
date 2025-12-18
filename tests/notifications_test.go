package tests

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/lib/notify"
	"github.com/neper-stars/neper/lib/stars"
	"github.com/neper-stars/neper/models"
	"github.com/neper-stars/neper/models/types"
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

	// Helper to get WebSocket URL from HTTP URL (handles both http:// and https://)
	wsURL := func(httpURL string) string {
		return "ws" + strings.TrimPrefix(httpURL, "http") + "/api/v1/notifications"
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
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
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
		defer func() { _ = gandalfConn.Close() }()

		merryConn := connectWS(merryToken)
		defer func() { _ = merryConn.Close() }()

		boromirConn := connectWS(boromirToken)
		defer func() { _ = boromirConn.Close() }()

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
		defer func() { _ = gandalfConn.Close() }()

		merryConn := connectWS(merryToken)
		defer func() { _ = merryConn.Close() }()

		boromirConn := connectWS(boromirToken)
		defer func() { _ = boromirConn.Close() }()

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

	t.Run("invited_user_receives_private_session_notifications", func(t *testing.T) {
		// isengardID is a private session
		// Merry is not a member but will be invited
		// After invitation, Merry should receive notifications for isengard

		gandalfToken := getToken("GandalfTheGrey", "apikeyGandalf")
		merryToken := getToken("Merry", "apikeyMerry")

		// First, make gandalf a manager of isengard so he can modify it
		tester.SetHeader("Authorization", "Bearer "+gandalfToken)
		// Join isengard as manager (gandalf is global manager so this works)
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/sessions/isengardID/join", JSONObj{
			"is_manager": true,
		}, nil))

		// Connect merry to WebSocket
		merryConn := connectWS(merryToken)
		defer func() { _ = merryConn.Close() }()

		var mu sync.Mutex
		merryNotifications := make([]*notify.ResourceChange, 0)

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

		wg.Add(1)
		go collectNotifications(merryConn, &merryNotifications, "merry")

		time.Sleep(100 * time.Millisecond)

		// Gandalf invites Merry to isengard (private session)
		var invite models.Invitation
		require.Equal(t, http.StatusCreated, tester.MustPostJSON("/api/v1/sessions/isengardID/invite", models.Invitation{
			SessionID:     "isengardID",
			UserProfileID: "merryID",
		}, &invite))

		// Now create rules for isengard - this triggers a session update notification
		var ruleset models.Ruleset
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/sessions/isengardID/rules", models.Ruleset{
			UniverseSize:     4,
			Density:          1,
			StartingDistance: 2,
			RandomSeed:       99999,
		}, &ruleset))

		wg.Wait()

		mu.Lock()
		defer mu.Unlock()

		// Find session update notification for isengardID
		var isengardSessionNotif *notify.ResourceChange
		for _, n := range merryNotifications {
			if n.Type == notify.TypeSession && n.ID == "isengardID" && n.Action == notify.ActionUpdated {
				isengardSessionNotif = n
				break
			}
		}

		require.NotNil(t, isengardSessionNotif, "merry (invited user) should receive private session notification for isengardID")
	})
}

func TestOrderStatusNotificationSentToAllSessionMembers(t *testing.T) {
	log := testutils.GetLogger(t)
	apiTesterConfigUpdater := NewAPITesterConfigUpdater(t, &log, true)
	tester := NewAPITester(t, apiTesterConfigUpdater.UpdateConfig)
	defer tester.Close()

	// Load fixtures for a 2-player session (merry vs gollum)
	tester.LoadFixtureFile("../restapi/handlers/fixtures/merryvsgollum.json")
	tester.LoadFixtureFile("../restapi/handlers/fixtures/merryvsgollum_turn0_files.json")

	server := tester.TestServer()
	defer server.Close()

	wsURL := func(httpURL string) string {
		return "ws" + strings.TrimPrefix(httpURL, "http") + "/api/v1/notifications"
	}

	getToken := func(nickname, apikey string) string {
		var token string
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/auth/authenticate", JSONObj{
			"nickname": nickname,
			"apikey":   apikey,
		}, &token))
		return token
	}

	connectWS := func(token string) *websocket.Conn {
		header := http.Header{}
		header.Set("Authorization", "Bearer "+token)
		conn, resp, err := websocket.DefaultDialer.Dial(wsURL(server.URL), header)
		require.NoError(t, err)
		require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
		return conn
	}

	readNotification := func(conn *websocket.Conn, timeout time.Duration) (*notify.ResourceChange, error) {
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
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

	t.Run("all_session_members_receive_order_status_notification_when_player_submits_turn", func(t *testing.T) {
		sessionID := "merryvsgollumID"
		year := 2400

		// Get tokens for both players
		merryToken := getToken("merry", "apikeyMerry")
		gollumToken := getToken("gollum", "apikeyGollum")

		// Connect both users to WebSocket
		merryConn := connectWS(merryToken)
		defer func() { _ = merryConn.Close() }()

		gollumConn := connectWS(gollumToken)
		defer func() { _ = gollumConn.Close() }()

		var mu sync.Mutex
		merryNotifications := make([]*notify.ResourceChange, 0)
		gollumNotifications := make([]*notify.ResourceChange, 0)

		var wg sync.WaitGroup

		collectNotifications := func(conn *websocket.Conn, notifications *[]*notify.ResourceChange, name string) {
			defer wg.Done()
			for {
				change, err := readNotification(conn, 3*time.Second)
				if err != nil {
					return
				}
				mu.Lock()
				*notifications = append(*notifications, change)
				t.Logf("%s received notification: type=%s, id=%s, action=%s", name, change.Type, change.ID, change.Action)
				mu.Unlock()
			}
		}

		wg.Add(2)
		go collectNotifications(merryConn, &merryNotifications, "merry")
		go collectNotifications(gollumConn, &gollumNotifications, "gollum")

		// Small delay to ensure WebSocket connections are established
		time.Sleep(100 * time.Millisecond)

		// Load gollum's turn file
		gollumOrderFileName := "../restapi/handlers/fixtures/merryvsgollum/Game.x2"
		gf, err := os.Open(gollumOrderFileName)
		require.NoError(t, err)
		gollumOrderContent, err := io.ReadAll(gf)
		require.NoError(t, err)
		gollumOrderContentB64 := stars.B64Encode(gollumOrderContent)

		// Gollum submits his turn
		tester.SetHeader("Authorization", "Bearer "+gollumToken)
		submitURL := fmt.Sprintf("/api/v1/sessions/%s/turn/%d", sessionID, year)
		turn := types.Order{B64Data: gollumOrderContentB64}
		var response interface{}
		require.Equal(t, http.StatusOK, tester.MustPutJSON(submitURL, &turn, &response))

		// Wait for notifications to be collected
		wg.Wait()

		mu.Lock()
		defer mu.Unlock()

		// Helper to find order_status notification for our session
		findOrderStatusNotification := func(notifications []*notify.ResourceChange) *notify.ResourceChange {
			for _, n := range notifications {
				if n.Type == notify.TypeOrderStatus && n.ID == sessionID {
					return n
				}
			}
			return nil
		}

		// Both merry and gollum should receive the order_status notification
		merryOrderStatus := findOrderStatusNotification(merryNotifications)
		gollumOrderStatus := findOrderStatusNotification(gollumNotifications)

		require.NotNil(t, merryOrderStatus, "merry (session member) should receive order_status notification when gollum submits")
		require.Equal(t, notify.ActionUpdated, merryOrderStatus.Action)
		merryMeta := notify.ParseMetadata[notify.OrderStatusMeta](merryOrderStatus)
		require.NotNil(t, merryMeta, "year should be present in metadata")
		require.Equal(t, int64(year), merryMeta.Year)

		require.NotNil(t, gollumOrderStatus, "gollum (submitter) should also receive order_status notification")
		require.Equal(t, notify.ActionUpdated, gollumOrderStatus.Action)
	})
}
