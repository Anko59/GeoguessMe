package integration_test

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// readWSText reads one text message from conn within the timeout and returns
// its content. A non-text frame or a read error fails the test.
func readWSText(t *testing.T, conn *websocket.Conn, timeout time.Duration) string {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(timeout)))
	_, payload, err := conn.ReadMessage()
	require.NoError(t, err)
	var msg struct {
		Kind    string `json:"kind"`
		Content string `json:"content"`
	}
	require.NoError(t, json.Unmarshal(payload, &msg))
	require.Equal(t, "text", msg.Kind)
	return msg.Content
}

// readUntilClosed blocks until conn reports a read error and fails the test if
// that error is only the read deadline elapsing: a closed socket surfaces a
// network error immediately while a live-but-silent socket stalls until the
// deadline. This distinguishes "closed by the peer" from "still open" without
// an unconditional sleep.
func readUntilClosed(t *testing.T, conn *websocket.Conn, timeout time.Duration) error {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(timeout)))
	_, _, err := conn.ReadMessage()
	require.Error(t, err, "expected the peer to close the connection")
	var netErr net.Error
	if errors.As(err, &netErr) {
		require.False(t, netErr.Timeout(), "expected the connection to close, not stall until the read deadline")
	}
	return err
}

// confirmLive exchanges a sentinel in both directions and drains every socket
// so no buffered frame can mask a later connection close. Returns the two live
// connections.
func confirmLive(t *testing.T, alice, bob *websocket.Conn) {
	t.Helper()
	require.NoError(t, alice.WriteJSON(map[string]string{"content": "alice-alive"}))
	require.Equal(t, "alice-alive", readWSText(t, alice, 5*time.Second)) // self echo
	require.Equal(t, "alice-alive", readWSText(t, bob, 5*time.Second))
	require.NoError(t, bob.WriteJSON(map[string]string{"content": "bob-alive"}))
	require.Equal(t, "bob-alive", readWSText(t, bob, 5*time.Second)) // self echo
	require.Equal(t, "bob-alive", readWSText(t, alice, 5*time.Second))
}

func TestStaleTicketFailsAfterLogoutAll(t *testing.T) {
	resetRateLimiter(t)
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	groupID, _ := createGroup(t, alice.access, "LogoutAll Group")
	ticket := wsTicket(t, alice.access, groupID)

	resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/logout?all=1", nil, "", []*http.Cookie{alice.refresh})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Logout-all must have deleted the outstanding ticket transactionally.
	db := testDB(t)
	var n int
	require.NoError(t, db.QueryRow(t.Context(), `SELECT COUNT(*) FROM websocket_tickets WHERE user_id = $1`, alice.userID).Scan(&n))
	require.Zero(t, n, "logout-all must delete outstanding WebSocket tickets")

	// The stale ticket must be rejected on consume.
	_, err := dialWS(t, groupID, ticket, baseURL)
	require.Error(t, err, "ticket minted before logout-all must be rejected")
}

func TestStaleTicketFailsAfterPasswordChange(t *testing.T) {
	resetRateLimiter(t)
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	groupID, _ := createGroup(t, alice.access, "PasswordChange Group")
	ticket := wsTicket(t, alice.access, groupID)

	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/password/change", map[string]string{
		"current_password": "StrongPassword123",
		"new_password":     "NewStrongPassword123",
	}, alice.access, nil)
	require.Equalf(t, http.StatusNoContent, resp.StatusCode, "password change: %s", data)

	_, err := dialWS(t, groupID, ticket, baseURL)
	require.Error(t, err, "ticket minted before a password change must be rejected")
}

func TestStaleTicketFailsAfterAccountDeletion(t *testing.T) {
	resetRateLimiter(t)
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	groupID, _ := createGroup(t, alice.access, "AccountDeletion Group")
	ticket := wsTicket(t, alice.access, groupID)

	resp, data := doJSON(t, http.MethodDelete, "/api/v1/auth/account",
		map[string]string{"password": "StrongPassword123"}, alice.access, nil)
	require.Equalf(t, http.StatusNoContent, resp.StatusCode, "account deletion: %s", data)

	_, err := dialWS(t, groupID, ticket, baseURL)
	require.Error(t, err, "ticket minted by a deleted account must be rejected")
}

func TestStaleTicketFailsAfterMembershipRemoval(t *testing.T) {
	resetRateLimiter(t)
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "MembershipRemoval Group")
	joinGroup(t, bob.access, code)

	// Sanity: a ticket minted while alice is a member is accepted.
	sanity := wsTicket(t, alice.access, groupID)
	conn, err := dialWS(t, groupID, sanity, baseURL)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	// No leave-group API exists yet, so remove alice's membership directly in
	// the test database. This will also be covered by a real leave-group API
	// when it lands; it exercises the same consume-time membership predicate.
	ticket := wsTicket(t, alice.access, groupID)
	db := testDB(t)
	_, err = db.Exec(t.Context(), `DELETE FROM group_members WHERE user_id = $1 AND group_id = $2`, alice.userID, groupID)
	require.NoError(t, err)

	_, err = dialWS(t, groupID, ticket, baseURL)
	require.Error(t, err, "ticket minted while a member must be rejected once membership is removed")
}

func TestEstablishedSocketClosesOnLogoutAll(t *testing.T) {
	resetRateLimiter(t)
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "SocketLogoutAll Group")
	joinGroup(t, bob.access, code)

	aliceConn := mustDialWS(t, groupID, wsTicket(t, alice.access, groupID), baseURL)
	defer aliceConn.Close()
	bobConn := mustDialWS(t, groupID, wsTicket(t, bob.access, groupID), baseURL)
	defer bobConn.Close()

	// Confirm both sockets are live, draining every socket's buffer.
	confirmLive(t, aliceConn, bobConn)

	// Logout-all revokes alice's credentials and must close her live socket.
	resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/logout?all=1", nil, "", []*http.Cookie{alice.refresh})
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	_ = readUntilClosed(t, aliceConn, 5*time.Second)
	require.NoError(t, aliceConn.SetWriteDeadline(time.Now().Add(2*time.Second)))
	var writeErr error
	for range 5 {
		if writeErr = aliceConn.WriteJSON(map[string]string{"content": "late"}); writeErr != nil {
			break
		}
	}
	require.Error(t, writeErr, "alice must not be able to send another event after logout-all")

	// Bob's socket is unaffected and stays live.
	require.NoError(t, bobConn.WriteJSON(map[string]string{"content": "bob-still-alive"}))
	require.Equal(t, "bob-still-alive", readWSText(t, bobConn, 5*time.Second))
}

func TestEstablishedSocketClosesOnPasswordChange(t *testing.T) {
	resetRateLimiter(t)
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "SocketPasswordChange Group")
	joinGroup(t, bob.access, code)

	aliceConn := mustDialWS(t, groupID, wsTicket(t, alice.access, groupID), baseURL)
	defer aliceConn.Close()
	bobConn := mustDialWS(t, groupID, wsTicket(t, bob.access, groupID), baseURL)
	defer bobConn.Close()

	// Confirm both sockets are live, draining every socket's buffer.
	confirmLive(t, aliceConn, bobConn)

	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/password/change", map[string]string{
		"current_password": "StrongPassword123",
		"new_password":     "NewStrongPassword123",
	}, alice.access, nil)
	require.Equalf(t, http.StatusNoContent, resp.StatusCode, "password change: %s", data)

	_ = readUntilClosed(t, aliceConn, 5*time.Second)

	// Bob's socket is unaffected and stays live.
	require.NoError(t, bobConn.WriteJSON(map[string]string{"content": "bob-still-alive"}))
	require.Equal(t, "bob-still-alive", readWSText(t, bobConn, 5*time.Second))
}
