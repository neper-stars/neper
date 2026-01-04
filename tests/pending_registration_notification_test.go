package tests

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"
	"orus.io/orus-io/go-orusapi/testutils"

	"github.com/neper-stars/neper/lib/notify"
)

func TestPendingUserReceivesApprovalNotification(t *testing.T) {
	log := testutils.GetLogger(t)
	apiTesterConfigUpdater := NewAPITesterConfigUpdater(t, &log, true)
	tester := NewAPITester(t, apiTesterConfigUpdater.UpdateConfig)
	defer tester.Close()

	// Load gandalf as global manager who can approve registrations
	tester.LoadFixtureFile("fixtures/gandalf_manager_only.json")
	// Load pending users for testing approval/rejection notifications
	tester.LoadFixtureFile("fixtures/pending_users.json")

	// Start the test server
	server := tester.TestServer()
	defer server.Close()

	// Helper to get WebSocket URL from HTTP URL
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

	// Helper to safely dereference string pointer
	ptrStr := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
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

	t.Run("pending_user_receives_approval_notification", func(t *testing.T) {
		// Get tokens for both users
		gandalfToken := getToken("GandalfTheGrey", "apikeyGandalf")
		pendingUserToken := getToken("PendingUser", "apikeyPending")

		// Connect both users to WebSocket
		gandalfConn := connectWS(gandalfToken)
		defer func() { _ = gandalfConn.Close() }()

		pendingUserConn := connectWS(pendingUserToken)
		defer func() { _ = pendingUserConn.Close() }()

		// Use mutex to safely collect notifications
		var mu sync.Mutex
		gandalfNotifications := make([]*notify.ResourceChange, 0)
		pendingUserNotifications := make([]*notify.ResourceChange, 0)

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
				t.Logf("%s received notification: type=%s, id=%s, action=%s", name, ptrStr(change.Type), ptrStr(change.ID), ptrStr(change.Action))
				mu.Unlock()
			}
		}

		wg.Add(2)
		go collectNotifications(gandalfConn, &gandalfNotifications, "gandalf")
		go collectNotifications(pendingUserConn, &pendingUserNotifications, "pendingUser")

		// Small delay to ensure WebSocket connections are established and subscriptions are active
		time.Sleep(100 * time.Millisecond)

		// Gandalf (global manager) approves the pending user
		tester.SetHeader("Authorization", "Bearer "+gandalfToken)
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/pending_registrations/pendingUserID/approve", nil, nil))

		// Wait for notifications to be collected
		wg.Wait()

		// Verify results
		mu.Lock()
		defer mu.Unlock()

		// Find approval notifications
		findApprovalNotification := func(notifications []*notify.ResourceChange, userID string) *notify.ResourceChange {
			for _, n := range notifications {
				if ptrStr(n.Type) == notify.ResourceChangeTypePendingRegistration &&
					ptrStr(n.ID) == userID &&
					ptrStr(n.Action) == notify.ResourceChangeActionApproved {
					return n
				}
			}
			return nil
		}

		gandalfApprovalNotif := findApprovalNotification(gandalfNotifications, "pendingUserID")
		pendingUserApprovalNotif := findApprovalNotification(pendingUserNotifications, "pendingUserID")

		// Gandalf is global manager, should see all notifications
		require.NotNil(t, gandalfApprovalNotif, "gandalf (global manager) should receive approval notification")

		// The pending user should receive their own approval notification
		require.NotNil(t, pendingUserApprovalNotif, "pending user should receive their own approval notification")
		require.Equal(t, notify.ResourceChangeActionApproved, ptrStr(pendingUserApprovalNotif.Action))

		// Verify metadata contains the user info
		meta := notify.ParseMetadata[notify.RegistrationApprovalMeta](pendingUserApprovalNotif)
		require.NotNil(t, meta, "approval notification should contain metadata")
		require.Equal(t, "pendingUserID", meta.UserProfileID)
		require.Equal(t, "PendingUser", meta.Nickname)
	})

	t.Run("pending_user_receives_rejection_notification", func(t *testing.T) {
		// Get tokens
		gandalfToken := getToken("GandalfTheGrey", "apikeyGandalf")
		rejectedUserToken := getToken("RejectedUser", "apikeyRejected")

		// Connect rejected user to WebSocket
		rejectedUserConn := connectWS(rejectedUserToken)
		defer func() { _ = rejectedUserConn.Close() }()

		var mu sync.Mutex
		rejectedUserNotifications := make([]*notify.ResourceChange, 0)

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
				t.Logf("%s received notification: type=%s, id=%s, action=%s", name, ptrStr(change.Type), ptrStr(change.ID), ptrStr(change.Action))
				mu.Unlock()
			}
		}

		wg.Add(1)
		go collectNotifications(rejectedUserConn, &rejectedUserNotifications, "rejectedUser")

		time.Sleep(100 * time.Millisecond)

		// Gandalf rejects the user
		tester.SetHeader("Authorization", "Bearer "+gandalfToken)
		require.Equal(t, http.StatusNoContent, tester.MustDoJSONRequest("DELETE", "/api/v1/pending_registrations/rejectedUserID/reject", nil, nil))

		wg.Wait()

		mu.Lock()
		defer mu.Unlock()

		// Find rejection notification
		var rejectionNotif *notify.ResourceChange
		for _, n := range rejectedUserNotifications {
			if ptrStr(n.Type) == notify.ResourceChangeTypePendingRegistration &&
				ptrStr(n.ID) == "rejectedUserID" &&
				ptrStr(n.Action) == notify.ResourceChangeActionRejected {
				rejectionNotif = n
				break
			}
		}

		require.NotNil(t, rejectionNotif, "rejected user should receive their own rejection notification")
	})

	t.Run("other_users_do_not_receive_approval_notification", func(t *testing.T) {
		// Get tokens
		gandalfToken := getToken("GandalfTheGrey", "apikeyGandalf")
		thirdPendingToken := getToken("ThirdPendingUser", "apikeyThird")
		merryToken := getToken("MerryRegular", "apikeyMerryRegular")

		// Connect all users to WebSocket
		thirdPendingConn := connectWS(thirdPendingToken)
		defer func() { _ = thirdPendingConn.Close() }()

		merryConn := connectWS(merryToken)
		defer func() { _ = merryConn.Close() }()

		var mu sync.Mutex
		thirdPendingNotifications := make([]*notify.ResourceChange, 0)
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
				t.Logf("%s received notification: type=%s, id=%s, action=%s", name, ptrStr(change.Type), ptrStr(change.ID), ptrStr(change.Action))
				mu.Unlock()
			}
		}

		wg.Add(2)
		go collectNotifications(thirdPendingConn, &thirdPendingNotifications, "thirdPending")
		go collectNotifications(merryConn, &merryNotifications, "merry")

		time.Sleep(100 * time.Millisecond)

		// Gandalf approves the third pending user
		tester.SetHeader("Authorization", "Bearer "+gandalfToken)
		require.Equal(t, http.StatusOK, tester.MustPostJSON("/api/v1/pending_registrations/thirdPendingUserID/approve", nil, nil))

		wg.Wait()

		mu.Lock()
		defer mu.Unlock()

		// The third pending user should receive their approval notification
		var thirdPendingApprovalNotif *notify.ResourceChange
		for _, n := range thirdPendingNotifications {
			if ptrStr(n.Type) == notify.ResourceChangeTypePendingRegistration &&
				ptrStr(n.ID) == "thirdPendingUserID" {
				thirdPendingApprovalNotif = n
				break
			}
		}
		require.NotNil(t, thirdPendingApprovalNotif, "third pending user should receive their own approval notification")

		// Merry (regular user, not the one being approved) should NOT receive the notification
		var merryApprovalNotif *notify.ResourceChange
		for _, n := range merryNotifications {
			if ptrStr(n.Type) == notify.ResourceChangeTypePendingRegistration {
				merryApprovalNotif = n
				break
			}
		}
		require.Nil(t, merryApprovalNotif, "merry (unrelated user) should NOT receive approval notification for another user")
	})
}
