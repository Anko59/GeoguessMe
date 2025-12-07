package integration_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebSocketAndUpload(t *testing.T) {
	// 1. Signup and Login
	username := "testuser_" + fmt.Sprint(time.Now().UnixNano())
	token, _ := signup(t, username, "TestPass123")

	// 2. Create Group
	groupID, _ := createGroup(t, token, "Test Group")

	// 3. Connect to WebSocket
	wsURL := "ws://localhost:8080/ws?group_id=" + groupID + "&token=" + token
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err, "WebSocket connection failed")
	defer ws.Close()

	// 4. Upload Photo
	uploadPhoto(t, token, groupID)

	// 5. Verify Message Received on WS
	_, msg, err := ws.ReadMessage()
	require.NoError(t, err, "Failed to read message from WS")
	fmt.Printf("Received WS Message: %s\n", msg)
	assert.Contains(t, string(msg), "NEW_PHOTO")
}
