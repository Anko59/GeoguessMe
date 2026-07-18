package integration_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func wsBase() string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "ws://localhost:8080"
	}
	return "ws://" + u.Host
}

func wsTicket(t *testing.T, bearer, groupID string) string {
	t.Helper()
	resp, data := doJSON(t, http.MethodPost, "/api/v1/ws/ticket?group_id="+groupID, nil, bearer, nil)
	require.Equalf(t, http.StatusCreated, resp.StatusCode, "ticket: %s", data)
	var result struct {
		Ticket string `json:"ticket"`
	}
	require.NoError(t, jsonUnmarshal(data, &result))
	require.NotEmpty(t, result.Ticket)
	return result.Ticket
}

func dialWS(t *testing.T, groupID, ticket, origin string) (*websocket.Conn, error) {
	t.Helper()
	header := http.Header{}
	if origin != "" {
		header.Set("Origin", origin)
	} else {
		header.Set("Origin", baseURL)
	}
	conn, response, err := websocket.DefaultDialer.Dial(wsBase()+"/api/v1/ws?group_id="+url.QueryEscape(groupID)+"&ticket="+url.QueryEscape(ticket), header)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	return conn, err
}

func mustDialWS(t *testing.T, groupID, ticket, origin string) *websocket.Conn {
	t.Helper()
	conn, err := dialWS(t, groupID, ticket, origin)
	require.NoError(t, err)
	return conn
}

func TestInvalidOriginDoesNotConsumeTicket(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	groupID, _ := createGroup(t, alice.access, "Origin Group")
	ticket := wsTicket(t, alice.access, groupID)

	// A disallowed origin must be rejected and must NOT burn the ticket.
	_, err := dialWS(t, groupID, ticket, "http://evil.example")
	require.Error(t, err)

	// The same ticket still works for an allowed origin.
	conn, err := dialWS(t, groupID, ticket, baseURL)
	require.NoError(t, err)
	defer conn.Close()
}

func TestTicketCannotBeReused(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	groupID, _ := createGroup(t, alice.access, "Ticket Group")
	ticket := wsTicket(t, alice.access, groupID)

	conn, err := dialWS(t, groupID, ticket, baseURL)
	require.NoError(t, err)
	_ = conn.Close()

	_, err = dialWS(t, groupID, ticket, baseURL)
	require.Error(t, err, "a consumed one-time ticket must be rejected")
}

func TestWebSocketBroadcastAndPersist(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Broadcast Group")
	joinGroup(t, bob.access, code)

	bobConn, err := dialWS(t, groupID, wsTicket(t, bob.access, groupID), baseURL)
	require.NoError(t, err)
	defer bobConn.Close()
	aliceConn, err := dialWS(t, groupID, wsTicket(t, alice.access, groupID), baseURL)
	require.NoError(t, err)
	defer aliceConn.Close()

	require.NoError(t, aliceConn.WriteJSON(map[string]string{"content": "hello world"}))

	require.NoError(t, bobConn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, payload, err := bobConn.ReadMessage()
	require.NoError(t, err)
	var msg struct {
		Content string `json:"content"`
		Kind    string `json:"kind"`
	}
	require.NoError(t, json.Unmarshal(payload, &msg))
	require.Equal(t, "hello world", msg.Content)
	require.Equal(t, "text", msg.Kind)

	// The message was persisted and is served by the REST endpoint.
	resp, data := doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID, nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.True(t, strings.Contains(string(data), "hello world"), "message must be persisted")
}

func TestWebSocketReconnectAndCatchUp(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Reconnect Group")
	joinGroup(t, bob.access, code)

	// ---- connect, send, remember cursor, disconnect ----
	aliceTicket1 := wsTicket(t, alice.access, groupID)
	aliceConn, err := dialWS(t, groupID, aliceTicket1, baseURL)
	require.NoError(t, err)

	require.NoError(t, aliceConn.WriteJSON(map[string]string{"content": "first"}))
	require.NoError(t, aliceConn.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, payload, err := aliceConn.ReadMessage()
	require.NoError(t, err)
	var firstMsg struct {
		ID        string `json:"id"`
		Content   string `json:"content"`
		CreatedAt string `json:"created_at"`
	}
	require.NoError(t, json.Unmarshal(payload, &firstMsg))
	require.Equal(t, "first", firstMsg.Content)

	createdAt, err := time.Parse(time.RFC3339Nano, firstMsg.CreatedAt)
	require.NoError(t, err)
	lastCursor := encodeMessageCursor(createdAt, firstMsg.ID)

	aliceConn.Close()

	// ---- create later messages while alice is disconnected ----
	bobConn, err := dialWS(t, groupID, wsTicket(t, bob.access, groupID), baseURL)
	require.NoError(t, err)
	defer bobConn.Close()

	require.NoError(t, bobConn.WriteJSON(map[string]string{"content": "second"}))
	require.NoError(t, bobConn.WriteJSON(map[string]string{"content": "third"}))
	for range 2 {
		require.NoError(t, bobConn.SetReadDeadline(time.Now().Add(5*time.Second)))
		_, _, err := bobConn.ReadMessage()
		require.NoError(t, err)
	}

	// ---- expired (consumed) ticket cannot be reused ----
	_, err = dialWS(t, groupID, aliceTicket1, baseURL)
	require.Error(t, err, "consumed one-time ticket must be rejected")

	// ---- renewed ticket connects successfully ----
	aliceTicket2 := wsTicket(t, alice.access, groupID)
	aliceConn2, err := dialWS(t, groupID, aliceTicket2, baseURL)
	require.NoError(t, err)
	defer aliceConn2.Close()

	// ---- catch up on missed messages from the last cursor ----
	resp, data := doJSON(t, http.MethodGet,
		"/api/v1/group/messages?group_id="+groupID+"&cursor="+url.QueryEscape(lastCursor),
		nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var page struct {
		Items []struct {
			ID      string `json:"id"`
			Content string `json:"content"`
		} `json:"items"`
	}
	require.NoError(t, jsonUnmarshal(data, &page))

	require.Len(t, page.Items, 2, "catch-up must return exactly the two missed messages")
	require.Equal(t, "second", page.Items[0].Content, "missed messages must be in chronological order")
	require.Equal(t, "third", page.Items[1].Content, "missed messages must be in chronological order")
	require.NotEqual(t, page.Items[0].ID, page.Items[1].ID, "each message must appear exactly once")
}

// encodeMessageCursor mirrors repository.encodeMessageCursor so integration
// tests can construct a reconnect cursor from a previously received message.
func encodeMessageCursor(createdAt time.Time, id string) string {
	payload := strconv.FormatInt(createdAt.UnixNano(), 10) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func TestWebSocketRejectsInvalidMessages(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Limits Group")
	joinGroup(t, bob.access, code)

	// readSystemError reads one text message from conn and asserts it
	// carries the expected system error code and content.
	readSystemError := func(conn *websocket.Conn, wantCode, wantContent string) {
		t.Helper()
		require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
		_, payload, err := conn.ReadMessage()
		require.NoError(t, err)
		var msg struct {
			Kind      string `json:"kind"`
			Content   string `json:"content"`
			ErrorCode string `json:"error_code"`
		}
		require.NoError(t, json.Unmarshal(payload, &msg))
		require.Equal(t, "system", msg.Kind)
		require.Equal(t, wantCode, msg.ErrorCode)
		require.Equal(t, wantContent, msg.Content)
	}

	// assertSentinel reads a valid text broadcast from observer and asserts
	// it matches the expected sentinel content. This proves the observer
	// connection is healthy and no invalid message was broadcast before the
	// sentinel arrived.
	assertSentinel := func(observer *websocket.Conn, wantContent string) {
		t.Helper()
		require.NoError(t, observer.SetReadDeadline(time.Now().Add(5*time.Second)))
		_, payload, err := observer.ReadMessage()
		require.NoError(t, err)
		var msg struct {
			Content string `json:"content"`
			Kind    string `json:"kind"`
		}
		require.NoError(t, json.Unmarshal(payload, &msg))
		require.Equal(t, "text", msg.Kind)
		require.Equal(t, wantContent, msg.Content)
	}

	// ---- whitespace-only content -------------------------------------------
	// Each invalid payload uses a fresh observer connection so the test
	// cases are independent. A valid sentinel written after the error
	// proves the observer never received the invalid message.
	bobObs := mustDialWS(t, groupID, wsTicket(t, bob.access, groupID), baseURL)
	defer bobObs.Close()
	aliceWS := mustDialWS(t, groupID, wsTicket(t, alice.access, groupID), baseURL)
	defer aliceWS.Close()

	require.NoError(t, aliceWS.WriteJSON(map[string]string{"content": "   "}))
	readSystemError(aliceWS, "invalid_message", "Message is empty or too long")
	require.NoError(t, aliceWS.WriteJSON(map[string]string{"content": "sentinel-ws"}))
	assertSentinel(bobObs, "sentinel-ws")

	// ---- malformed JSON ----------------------------------------------------
	bobObs2 := mustDialWS(t, groupID, wsTicket(t, bob.access, groupID), baseURL)
	defer bobObs2.Close()
	aliceWS2 := mustDialWS(t, groupID, wsTicket(t, alice.access, groupID), baseURL)
	defer aliceWS2.Close()

	require.NoError(t, aliceWS2.WriteMessage(websocket.TextMessage, []byte(`{not json}`)))
	readSystemError(aliceWS2, "invalid_message", "Message must contain text")
	require.NoError(t, aliceWS2.WriteJSON(map[string]string{"content": "sentinel-json"}))
	assertSentinel(bobObs2, "sentinel-json")

	// ---- content exceeding maxTextLength (1000 runes) ----------------------
	bobObs3 := mustDialWS(t, groupID, wsTicket(t, bob.access, groupID), baseURL)
	defer bobObs3.Close()
	aliceWS3 := mustDialWS(t, groupID, wsTicket(t, alice.access, groupID), baseURL)
	defer aliceWS3.Close()

	longContent := strings.Repeat("é", 1001)
	raw, err := json.Marshal(map[string]string{"content": longContent})
	require.NoError(t, err)
	require.NoError(t, aliceWS3.WriteMessage(websocket.TextMessage, raw))
	readSystemError(aliceWS3, "invalid_message", "Message is empty or too long")
	require.NoError(t, aliceWS3.WriteJSON(map[string]string{"content": "sentinel-long"}))
	assertSentinel(bobObs3, "sentinel-long")

	// ---- oversized message exceeds 4096-byte read limit --------------------
	// The server closes the connection when the read limit is exceeded, so
	// we must dial a fresh sender to send the sentinel proof afterwards.
	bobObs4 := mustDialWS(t, groupID, wsTicket(t, bob.access, groupID), baseURL)
	defer bobObs4.Close()
	aliceWS4 := mustDialWS(t, groupID, wsTicket(t, alice.access, groupID), baseURL)
	defer aliceWS4.Close()

	big := strings.Repeat("x", 5000)
	_ = aliceWS4.WriteMessage(websocket.TextMessage, []byte(`{"content":"`+big+`"}`))
	require.NoError(t, aliceWS4.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, _, err = aliceWS4.ReadMessage()
	require.Error(t, err, "connection must close after oversized message")
	// The server does not send an application-level close frame for a read-
	// limit violation; it just tears down the connection. Verify the
	// connection is truly dead.
	require.NoError(t, aliceWS4.SetReadDeadline(time.Now().Add(100*time.Millisecond)))
	_, _, err = aliceWS4.ReadMessage()
	require.Error(t, err, "connection must remain closed")

	// Prove no invalid broadcast: a new sender sends a valid sentinel that
	// the observer must receive.
	aliceWS4b := mustDialWS(t, groupID, wsTicket(t, alice.access, groupID), baseURL)
	defer aliceWS4b.Close()
	require.NoError(t, aliceWS4b.WriteJSON(map[string]string{"content": "sentinel-oversized"}))
	assertSentinel(bobObs4, "sentinel-oversized")

	// ---- prove nothing invalid was persisted -------------------------------
	resp, data := doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var page struct {
		Items []struct {
			Content string `json:"content"`
		} `json:"items"`
	}
	require.NoError(t, jsonUnmarshal(data, &page))
	// Only the four valid sentinels must be persisted; no invalid message
	// content must appear. The sentinels prove every observer path was live.
	sentinels := map[string]bool{
		"sentinel-ws":        true,
		"sentinel-json":      true,
		"sentinel-long":      true,
		"sentinel-oversized": true,
	}
	require.Len(t, page.Items, len(sentinels), "only valid sentinel messages must be persisted")
	for _, item := range page.Items {
		require.True(t, sentinels[item.Content], "unexpected persisted message: %s", item.Content)
	}
}
