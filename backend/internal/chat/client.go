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
)

type Client struct {
	hub     *Hub
	conn    *websocket.Conn
	send    chan models.Message
	groupID string
	userID  string
}

type incomingMessage struct {
	Content string `json:"content"`
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
		c.hub.BroadcastFrom(c, models.Message{GroupID: c.groupID, UserID: c.userID, Kind: "text", Content: input.Content, CreatedAt: time.Now()})
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() { ticker.Stop(); _ = c.conn.Close() }()
	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteJSON(message); err != nil {
				return
			}
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
	client := &Client{hub: hub, conn: conn, send: make(chan models.Message, 64), groupID: groupID, userID: userID}
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
