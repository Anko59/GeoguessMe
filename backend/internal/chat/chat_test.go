package chat

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/gorilla/websocket"
)

func TestOriginAllowed(t *testing.T) {
	if !OriginAllowed("", []string{"https://example.test"}) {
		t.Fatal("empty origin should be accepted for non-browser clients")
	}
	if !OriginAllowed("https://example.test", []string{"https://example.test"}) {
		t.Fatal("configured origin was rejected")
	}
	if OriginAllowed("https://evil.test", []string{"https://example.test"}) {
		t.Fatal("unconfigured origin was accepted")
	}
}

func TestHubPersistsAndBroadcastsByGroup(t *testing.T) {
	persisted := make(chan models.Message, 1)
	hub := NewHub(func(_ context.Context, message *models.Message) error {
		persisted <- *message
		return nil
	}, nil)
	go hub.Run()
	first := &Client{groupID: "group-a", send: make(chan models.Message, 1)}
	second := &Client{groupID: "group-b", send: make(chan models.Message, 1)}
	hub.register <- first
	hub.register <- second
	hub.Broadcast(models.Message{GroupID: "group-a", Content: "hello"})

	select {
	case message := <-persisted:
		if message.ID == "" || message.Kind != "text" || message.CreatedAt.IsZero() {
			t.Fatalf("persisted defaults missing: %+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("message was not persisted")
	}
	select {
	case message := <-first.send:
		if message.Content != "hello" || message.GroupID != "group-a" {
			t.Fatalf("unexpected broadcast: %+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("matching client did not receive message")
	}
	select {
	case message := <-second.send:
		t.Fatalf("message leaked to another group: %+v", message)
	case <-time.After(20 * time.Millisecond):
	}
	hub.Stop()
}

func TestHubReportsPersistenceFailureToSender(t *testing.T) {
	hub := NewHub(func(context.Context, *models.Message) error { return errors.New("database offline") }, nil)
	go hub.Run()
	sender := &Client{groupID: "group-a", send: make(chan models.Message, 1)}
	hub.register <- sender
	hub.BroadcastFrom(sender, models.Message{GroupID: "group-a", Content: "not saved"})
	select {
	case message := <-sender.send:
		if message.Kind != "system" || message.ErrorCode != "message_not_saved" {
			t.Fatalf("unexpected failure message: %+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("sender was not notified")
	}
	hub.Stop()
}

// TestHubNotifiesOnlyForTextMessages asserts the push callback fires for chat
// messages but not for challenge broadcasts (notified by the upload handler) or
// persistence failures.
func TestHubNotifiesOnlyForTextMessages(t *testing.T) {
	notified := make(chan models.Message, 4)
	hub := NewHub(func(context.Context, *models.Message) error { return nil }, func(_ context.Context, msg *models.Message) {
		notified <- *msg
	})
	go hub.Run()
	hub.Broadcast(models.Message{GroupID: "group-a", Kind: "text", Content: "hi"})
	challengeID := "photo-1"
	hub.Broadcast(models.Message{GroupID: "group-a", Kind: "challenge", PhotoID: &challengeID})
	select {
	case message := <-notified:
		if message.Kind != "text" || message.Content != "hi" {
			t.Fatalf("only text messages should be notified, got %+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("text message was not notified")
	}
	select {
	case message := <-notified:
		t.Fatalf("challenge broadcast was notified: %+v", message)
	case <-time.After(40 * time.Millisecond):
	}
	hub.Stop()
}

func TestServeWsValidatesMessagesAndBroadcasts(t *testing.T) {
	persisted := make(chan models.Message, 1)
	hub := NewHub(func(_ context.Context, message *models.Message) error {
		persisted <- *message
		return nil
	}, nil)
	go hub.Run()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWs(hub, w, r, "group-a", "user-a", []string{"http://allowed.test"})
	}))
	defer server.Close()

	dialer := websocket.Dialer{}
	url := "ws" + server.URL[len("http"):] + "/ws"
	conn, response, err := dialer.Dial(url, http.Header{"Origin": []string{"http://allowed.test"}})
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("status = %d", response.StatusCode)
	}
	defer conn.Close()
	if err := conn.WriteJSON(map[string]string{"content": "   "}); err != nil {
		t.Fatal(err)
	}
	_, _, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("reading validation response: %v", err)
	}
	if err := conn.WriteJSON(map[string]string{"content": "hello over websocket"}); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-persisted:
		if message.Content != "hello over websocket" || message.UserID != "user-a" {
			t.Fatalf("unexpected persisted websocket message: %+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("valid message was not persisted")
	}
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("reading broadcast: %v", err)
	}
	if string(payload) == "" {
		t.Fatal("empty websocket broadcast")
	}
	hub.Stop()
}
