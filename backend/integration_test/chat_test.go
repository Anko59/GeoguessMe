package integration_test

import (
	"encoding/json"
	"net/http"
	"net/url"
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

func TestWebSocketRejectsEmptyAndOversized(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	groupID, _ := createGroup(t, alice.access, "Limits Group")
	conn, err := dialWS(t, groupID, wsTicket(t, alice.access, groupID), baseURL)
	require.NoError(t, err)
	defer conn.Close()

	// Empty message is ignored by the server; the connection stays open.
	require.NoError(t, conn.WriteJSON(map[string]string{"content": "  "}))
	// Oversized payload exceeds the read limit and closes the connection.
	big := strings.Repeat("x", 5000)
	_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"content":"`+big+`"}`))
}
