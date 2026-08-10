package chat

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"geoguessme/internal/models"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 9 * pongWait / 10
	maxMessageSize = 4096
	maxTextLength  = 1000
	// revalidateEvery bounds how often a socket's validity is re-checked
	// against the revalidator before an incoming message is accepted. It is
	// shorter than the hub's periodic sweep so a revoked user cannot keep
	// sending for up to a full sweep interval.
	revalidateEvery = 5 * time.Second
)

type Client struct {
	hub         *Hub
	conn        *websocket.Conn
	send        chan models.Message
	done        chan struct{}
	groupID     string
	userID      string
	valid       bool
	validatedAt time.Time
}

type incomingMessage struct {
	Content   string  `json:"content"`
	ReplyToID *string `json:"reply_to_id,omitempty"`
}

func (c *Client) readPump() {
	defer func() { c.hub.unregister <- c; _ = c.conn.Close() }()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { return c.conn.SetReadDeadline(time.Now().Add(pongWait)) })
	for {
		_, payload, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var input incomingMessage
		decoder := json.NewDecoder(strings.NewReader(string(payload)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || !utf8.ValidString(input.Content) {
			sendSystem(c, "invalid_message", "Message must contain text")
			continue
		}
		input.Content = strings.TrimSpace(input.Content)
		if input.Content == "" || utf8.RuneCountInString(input.Content) > maxTextLength {
			sendSystem(c, "invalid_message", "Message is empty or too long")
			continue
		}
		// Revalidate before accepting the message: a user who was revoked or
		// removed from the group since the socket was validated must not be
		// able to send another event. Returning closes the connection via the
		// deferred unregister.
		if !c.ensureValid() {
			return
		}
		c.hub.BroadcastFrom(c, models.Message{GroupID: c.groupID, UserID: c.userID, Kind: "text", ReplyToID: input.ReplyToID, Content: input.Content, CreatedAt: time.Now()})
	}
}

// ensureValid reports whether the client may keep exchanging messages. The
// first check always consults the hub revalidator; subsequent checks within
// revalidateEvery reuse the cached result so a busy chat cannot hit the
// database once per message.
func (c *Client) ensureValid() bool {
	if c.valid && time.Since(c.validatedAt) <= revalidateEvery {
		return true
	}
	c.valid = c.hub.revalidateClient(c)
	c.validatedAt = time.Now()
	return c.valid
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() { ticker.Stop(); _ = c.conn.Close() }()
	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Defensive: the hub never closes send (teardown runs through the
				// done channel), but a closed channel must still terminate the
				// pump cleanly.
				return
			}
			if err := c.conn.WriteJSON(message); err != nil {
				return
			}
		case <-c.done:
			// The hub removed the socket (credential revocation, failed
			// revalidation, or a slow consumer). Attempt a clean close frame,
			// then let the deferred close tear the connection down. The send
			// channel is never closed by the hub, so concurrent sendSystem
			// calls from readPump can never panic on a closed channel.
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
			return
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request, groupID, userID string, allowedOrigins []string) {
	upgrader := websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024, CheckOrigin: func(request *http.Request) bool { return OriginAllowed(request.Header.Get("Origin"), allowedOrigins) }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("websocket upgrade failed", "error", err)
		return
	}
	client := &Client{hub: hub, conn: conn, send: make(chan models.Message, 64), done: make(chan struct{}), groupID: groupID, userID: userID}
	hub.register <- client
	go client.writePump()
	go client.readPump()
}

// OriginAllowed reports whether a WebSocket upgrade Origin is permitted.
func OriginAllowed(origin string, allowed []string) bool {
	if origin == "" {
		return true
	}
	for _, candidate := range allowed {
		if candidate == origin {
			return true
		}
	}
	return false
}
