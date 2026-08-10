package chat

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
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

func TestHubDisconnectUserClosesThatUsersSocketsOnly(t *testing.T) {
	hub := NewHub(nil, nil)
	go hub.Run()
	t.Cleanup(hub.Stop)

	aliceInA := &Client{userID: "alice", groupID: "group-a", send: make(chan models.Message, 1), done: make(chan struct{})}
	aliceInB := &Client{userID: "alice", groupID: "group-b", send: make(chan models.Message, 1), done: make(chan struct{})}
	bobInA := &Client{userID: "bob", groupID: "group-a", send: make(chan models.Message, 1), done: make(chan struct{})}
	hub.register <- aliceInA
	hub.register <- aliceInB
	hub.register <- bobInA

	hub.DisconnectUser("alice")

	waitSendClosed(t, aliceInA, "alice/group-a socket was not disconnected")
	waitSendClosed(t, aliceInB, "alice/group-b socket was not disconnected")
	waitSendOpen(t, bobInA, "bob's socket was disconnected")
}

func TestHubDisconnectUserInGroupClosesOnlyMatchingSocket(t *testing.T) {
	hub := NewHub(nil, nil)
	go hub.Run()
	t.Cleanup(hub.Stop)

	aliceInA := &Client{userID: "alice", groupID: "group-a", send: make(chan models.Message, 1), done: make(chan struct{})}
	aliceInB := &Client{userID: "alice", groupID: "group-b", send: make(chan models.Message, 1), done: make(chan struct{})}
	hub.register <- aliceInA
	hub.register <- aliceInB

	hub.DisconnectUserInGroup("alice", "group-a")

	waitSendClosed(t, aliceInA, "matching socket was not disconnected")
	waitSendOpen(t, aliceInB, "socket in another group was disconnected")
}

func TestHubRevalidationClosesInvalidSockets(t *testing.T) {
	hub := NewHub(nil, nil)
	hub.Revalidate = func(userID, groupID string) bool { return userID != "revoked" }
	go hub.Run()
	t.Cleanup(hub.Stop)

	valid := &Client{userID: "ok", groupID: "group-a", send: make(chan models.Message, 1), done: make(chan struct{})}
	invalid := &Client{userID: "revoked", groupID: "group-a", send: make(chan models.Message, 1), done: make(chan struct{})}
	hub.register <- valid
	hub.register <- invalid

	hub.RevalidateNow()

	waitSendClosed(t, invalid, "invalid socket survived revalidation")
	waitSendOpen(t, valid, "valid socket was closed by revalidation")
}

func TestHubRevalidationDoesNotBlockDisconnects(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	hub := NewHub(nil, nil)
	hub.Revalidate = func(userID, groupID string) bool {
		if userID == "slow" {
			close(started)
			<-release
		}
		return true
	}
	go hub.Run()
	t.Cleanup(hub.Stop)

	slow := &Client{userID: "slow", groupID: "group-a", send: make(chan models.Message, 1), done: make(chan struct{})}
	revoked := &Client{userID: "revoked", groupID: "group-a", send: make(chan models.Message, 1), done: make(chan struct{})}
	hub.register <- slow
	hub.register <- revoked
	hub.RevalidateNow()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("slow revalidation did not start")
	}

	// The hub must continue handling explicit revocations while a repository
	// check is blocked. The old synchronous sweep deadlocked here until release.
	hub.DisconnectUser("revoked")
	waitSendClosed(t, revoked, "disconnect stalled behind socket revalidation")
	close(release)
}

func TestUnregisterClientReturnsAfterHubStops(t *testing.T) {
	hub := NewHub(nil, nil)
	go hub.Run()
	hub.Stop()

	returned := make(chan struct{})
	go func() {
		hub.unregisterClient(&Client{})
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("unregister blocked after the hub stopped")
	}
}

func TestClientRevalidationCachesWithinTTLAndRechecksAfter(t *testing.T) {
	hub := NewHub(nil, nil)
	var calls int
	hub.Revalidate = func(userID, groupID string) bool {
		calls++
		return true
	}
	client := &Client{hub: hub, userID: "user-a", groupID: "group-a"}
	if !client.ensureValid() {
		t.Fatal("fresh client failed revalidation")
	}
	if !client.ensureValid() {
		t.Fatal("cached valid client failed revalidation")
	}
	if calls != 1 {
		t.Fatalf("revalidate calls = %d, want 1 (cache must be honored within the TTL)", calls)
	}
	client.validatedAt = time.Now().Add(-10 * time.Second)
	if !client.ensureValid() {
		t.Fatal("client failed revalidation after cache expiry")
	}
	if calls != 2 {
		t.Fatalf("revalidate calls = %d, want 2 (cache must expire after the TTL)", calls)
	}
}

func TestClientWithoutRevalidatorIsAlwaysValid(t *testing.T) {
	hub := NewHub(nil, nil)
	client := &Client{hub: hub, userID: "user-a", groupID: "group-a"}
	for i := 0; i < 3; i++ {
		if !client.ensureValid() {
			t.Fatal("client without a revalidator must remain valid")
		}
	}
}

// waitSendClosed blocks until the client's done channel is closed (which the
// hub does when it disconnects a socket) or fails after a bounded wait.
func waitSendClosed(t *testing.T, client *Client, message string) {
	t.Helper()
	select {
	case <-client.done:
	case <-time.After(time.Second):
		t.Fatalf("%s: socket was not disconnected", message)
	}
}

// waitSendOpen fails if the client's done channel closes within a short
// window, proving a socket was left connected.
func waitSendOpen(t *testing.T, client *Client, message string) {
	t.Helper()
	select {
	case <-client.done:
		t.Fatalf("%s: socket was disconnected", message)
	case <-time.After(20 * time.Millisecond):
	}
}

// TestSendSystemAfterDisconnectDoesNotPanic pins the F-03 close-safety
// invariant: a system message sent after the hub has removed the socket must
// not panic. Before the done-channel redesign, remove() closed the send
// channel and sendSystem's select send to the closed channel panicked (a
// closed channel is always selectable), which a slow or malicious peer could
// trigger on any disconnect path (logout-all, revalidation, slow-consumer
// drop). The fix leaves the send channel open forever: a post-removal send is
// a harmless non-blocking send into an unread buffer.
func TestSendSystemAfterDisconnectDoesNotPanic(t *testing.T) {
	hub := NewHub(nil, nil)
	go hub.Run()
	t.Cleanup(hub.Stop)

	client := &Client{userID: "alice", groupID: "group-a", send: make(chan models.Message, 4), done: make(chan struct{})}
	hub.register <- client
	hub.DisconnectUser("alice")
	waitSendClosed(t, client, "socket was not disconnected")

	// The send channel must remain open after removal: the queued system
	// message is delivered and nothing panics.
	sendSystem(client, "invalid_message", "too late")
	select {
	case message, ok := <-client.send:
		if !ok {
			t.Fatal("remove() closed the send channel; post-disconnect sends would panic")
		}
		if message.ErrorCode != "invalid_message" {
			t.Fatalf("unexpected queued message: %+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("post-disconnect sendSystem did not queue its message")
	}
}

// TestSendSystemConcurrentWithDisconnectDoesNotPanic hammers sendSystem from a
// reader goroutine while the hub disconnects the socket, exercising the exact
// interleaving that previously panicked (send-on-closed-channel). Teardown is
// synchronized with a stop channel and a WaitGroup rather than sleeps;
// together with TestSendSystemAfterDisconnectDoesNotPanic it pins the
// invariant that the send channel is never closed by the hub.
func TestSendSystemConcurrentWithDisconnectDoesNotPanic(t *testing.T) {
	hub := NewHub(nil, nil)
	go hub.Run()
	t.Cleanup(hub.Stop)

	client := &Client{userID: "alice", groupID: "group-a", send: make(chan models.Message, 4), done: make(chan struct{})}
	hub.register <- client

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				sendSystem(client, "invalid_message", "raced")
			}
		}
	}()
	hub.DisconnectUser("alice")
	// Keep the sender hammering until the disconnect is observed so the send
	// loop is guaranteed to race with the hub's removal. Pre-fix (remove()
	// closed the send channel) this interleaving crashes the process with
	// "panic: send on closed channel".
	waitSendClosed(t, client, "socket was not disconnected")
	close(stop)
	wg.Wait()
}

func TestHubPersistsAndBroadcastsByGroup(t *testing.T) {
	persisted := make(chan models.Message, 1)
	hub := NewHub(func(_ context.Context, message *models.Message) error {
		persisted <- *message
		return nil
	}, nil)
	go hub.Run()
	first := &Client{groupID: "group-a", send: make(chan models.Message, 1), done: make(chan struct{})}
	second := &Client{groupID: "group-b", send: make(chan models.Message, 1), done: make(chan struct{})}
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
	sender := &Client{groupID: "group-a", send: make(chan models.Message, 1), done: make(chan struct{})}
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

// TestHubPersistTimeoutBoundsHungPersistence pins the bounded-persist
// invariant: the hub Run loop is single-threaded, so a database write that
// hangs must fail within the persist deadline instead of stalling every
// broadcast forever. The fake persist honors context cancellation exactly like
// the pgx-backed save (it returns when the context expires); the hub must
// report the failure to the sender and keep processing later broadcasts. This
// test fails against an unbounded persist call because the first save would
// block on the never-cancelled background context and the sender would never
// receive the failure message.
func TestHubPersistTimeoutBoundsHungPersistence(t *testing.T) {
	var mu sync.Mutex
	var calls int
	persist := func(ctx context.Context, _ *models.Message) error {
		mu.Lock()
		first := calls == 0
		calls++
		mu.Unlock()
		if first {
			// Simulate a database call that hangs but honors cancellation
			// (pgx aborts the query when the context expires).
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	hub := NewHubWithTimeout(persist, nil, 30*time.Millisecond)
	go hub.Run()
	sender := &Client{groupID: "group-a", send: make(chan models.Message, 4), done: make(chan struct{})}
	hub.register <- sender
	defer hub.Stop()

	// The first persist hangs; the hub must bound it and report the dropped
	// message to the sender instead of wedging the loop.
	hub.BroadcastFrom(sender, models.Message{GroupID: "group-a", Content: "stuck"})
	select {
	case message := <-sender.send:
		if message.Kind != "system" || message.ErrorCode != "message_not_saved" {
			t.Fatalf("unexpected failure message: %+v", message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hub stalled: hung persistence was not bounded by the persist timeout")
	}

	// The hub loop is still alive: a later broadcast persists successfully and
	// reaches the sender.
	hub.BroadcastFrom(sender, models.Message{GroupID: "group-a", Content: "second"})
	select {
	case message := <-sender.send:
		if message.Content != "second" {
			t.Fatalf("unexpected second message: %+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("hub did not continue processing broadcasts after a bounded persist failure")
	}
}

// TestHubNotifiesOnlyForChatMessages asserts the push callback fires for text
// and media chat messages but not for challenges (notified by their handler).
func TestHubNotifiesOnlyForChatMessages(t *testing.T) {
	notified := make(chan models.Message, 4)
	hub := NewHub(func(context.Context, *models.Message) error { return nil }, func(_ context.Context, msg *models.Message) {
		notified <- *msg
	})
	go hub.Run()
	hub.Broadcast(models.Message{GroupID: "group-a", Kind: "text", Content: "hi"})
	hub.BroadcastPersisted(models.Message{GroupID: "group-a", Kind: "media", Content: "photo"})
	challengeID := "photo-1"
	hub.Broadcast(models.Message{GroupID: "group-a", Kind: "challenge", PhotoID: &challengeID})
	select {
	case message := <-notified:
		if message.Kind != "text" || message.Content != "hi" {
			t.Fatalf("text message was not notified, got %+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("text message was not notified")
	}
	select {
	case message := <-notified:
		if message.Kind != "media" || message.Content != "photo" {
			t.Fatalf("media message was not notified: %+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("media message was not notified")
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
	if err := conn.WriteJSON(map[string]string{"content": "hello over websocket", "reply_to_id": "parent-message"}); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-persisted:
		if message.Content != "hello over websocket" || message.UserID != "user-a" || message.ReplyToID == nil || *message.ReplyToID != "parent-message" {
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
