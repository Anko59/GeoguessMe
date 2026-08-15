package chat

import (
	"context"
	"sync"
	"time"

	"geoguessme/internal/models"

	"github.com/google/uuid"
)

type PersistFunc func(context.Context, *models.Message) error

// NotifyFunc is invoked after a message is persisted and broadcast. It is used
// to fan push notifications for new chat messages; the callback must be
// non-blocking. It receives the final message (with ID/timestamp assigned).
type NotifyFunc func(context.Context, *models.Message)

type event struct {
	message          models.Message
	sender           *Client
	alreadyPersisted bool
	notify           bool
}

// RevalidateFunc reports whether a live socket for userID in groupID may stay
// connected. It is invoked periodically and again before an incoming message
// is accepted; returning false closes the socket. A nil revalidator means
// every socket stays connected (the hub then relies solely on explicit
// revocation disconnects).
type RevalidateFunc func(userID, groupID string) bool

// SocketKicker closes live WebSocket clients. The hub implements it, and
// handlers that revoke credentials use it so sockets minted under a
// now-revoked auth version close promptly.
type SocketKicker interface {
	DisconnectUser(userID string)
	DisconnectUserInGroup(userID, groupID string)
}

// userGroupPair targets a socket disconnect at one user within one group.
type userGroupPair struct {
	userID  string
	groupID string
}

type revalidationResult struct {
	client *Client
	valid  bool
}

type Hub struct {
	clients             map[*Client]bool
	clientsByUser       map[string]map[*Client]bool
	broadcast           chan event
	register            chan *Client
	unregister          chan *Client
	disconnectUser      chan string
	disconnectUserGroup chan userGroupPair
	revalidateNow       chan struct{}
	revalidationResults chan revalidationResult
	revalidationDone    chan struct{}
	stop                chan struct{}
	stopped             chan struct{}
	persist             PersistFunc
	notify              NotifyFunc
	persistTimeout      time.Duration
	// Revalidate is the live-socket validity check run periodically and
	// before each incoming message is accepted. It is injected by the
	// composition root (closing over the repository) after construction;
	// nil disables revalidation.
	Revalidate         RevalidateFunc
	revalidateInterval time.Duration
	once               sync.Once
}

// defaultPersistTimeout bounds a single persistence call. The hub Run loop is
// single-threaded, so an unbounded persist could stall every broadcast; the
// deadline makes a hung database call fail instead of wedging the hub (the
// context is honored by the pgx-backed SaveMessage path).
const defaultPersistTimeout = 5 * time.Second

// NewHub builds a hub that bounds every persistence call with the default
// five-second deadline.
func NewHub(persist PersistFunc, notify NotifyFunc) *Hub {
	return NewHubWithTimeout(persist, notify, defaultPersistTimeout)
}

// defaultRevalidateInterval is how often the hub re-checks every live socket
// against the configured revalidator. It is deliberately longer than the
// per-message cache window so a busy group cannot hammer the database.
const defaultRevalidateInterval = 30 * time.Second

// NewHubWithTimeout builds a hub with an explicit persistence deadline. It is
// the constructor tests use to exercise the bounded-persist invariant with a
// tiny timeout instead of waiting five seconds. A non-positive timeout falls
// back to the default so a hub can never be accidentally unbounded.
func NewHubWithTimeout(persist PersistFunc, notify NotifyFunc, persistTimeout time.Duration) *Hub {
	if persistTimeout <= 0 {
		persistTimeout = defaultPersistTimeout
	}
	return &Hub{
		broadcast:           make(chan event, 128),
		register:            make(chan *Client),
		unregister:          make(chan *Client),
		disconnectUser:      make(chan string, 64),
		disconnectUserGroup: make(chan userGroupPair, 64),
		revalidateNow:       make(chan struct{}, 1),
		revalidationResults: make(chan revalidationResult, 128),
		revalidationDone:    make(chan struct{}, 1),
		clients:             make(map[*Client]bool),
		clientsByUser:       make(map[string]map[*Client]bool),
		stop:                make(chan struct{}),
		stopped:             make(chan struct{}),
		persist:             persist,
		notify:              notify,
		persistTimeout:      persistTimeout,
		revalidateInterval:  defaultRevalidateInterval,
	}
}

func (h *Hub) Run() {
	defer close(h.stopped)
	revalidateTicker := time.NewTicker(h.revalidateInterval)
	defer revalidateTicker.Stop()
	revalidationRunning := false
	revalidationPending := false
	requestRevalidation := func() {
		if revalidationRunning {
			revalidationPending = true
			return
		}
		revalidationRunning = h.startRevalidation()
	}
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			if h.clientsByUser[client.userID] == nil {
				h.clientsByUser[client.userID] = make(map[*Client]bool)
			}
			h.clientsByUser[client.userID][client] = true
		case client := <-h.unregister:
			h.remove(client)
		case userID := <-h.disconnectUser:
			h.disconnectSockets(userID, "")
		case pair := <-h.disconnectUserGroup:
			h.disconnectSockets(pair.userID, pair.groupID)
		case <-h.revalidateNow:
			requestRevalidation()
		case <-revalidateTicker.C:
			requestRevalidation()
		case result := <-h.revalidationResults:
			if !result.valid {
				h.remove(result.client)
			}
		case <-h.revalidationDone:
			revalidationRunning = false
			if revalidationPending {
				revalidationPending = false
				requestRevalidation()
			}
		case incoming := <-h.broadcast:
			message := incoming.message
			if message.ID == "" {
				message.ID = newMessageID()
			}
			if message.CreatedAt.IsZero() {
				message.CreatedAt = time.Now()
			}
			if message.Kind == "" {
				message.Kind = "text"
			}
			if !incoming.alreadyPersisted && h.persist != nil {
				// Bound the persistence call so a hung database cannot stall the
				// single-threaded hub. The deadline is honored by the pgx-backed
				// save; on expiry the error path below drops the message and the
				// loop keeps serving every other broadcast.
				ctx, cancel := context.WithTimeout(context.Background(), h.persistTimeout)
				err := h.persist(ctx, &message)
				cancel()
				if err != nil {
					if incoming.sender != nil {
						sendSystem(incoming.sender, "message_not_saved", "Message could not be sent")
					}
					continue
				}
			}
			for client := range h.clients {
				if client.groupID != message.GroupID {
					continue
				}
				select {
				case client.send <- message:
				default:
					h.remove(client)
				}
			}
			// Fan a push notification for ordinary chat messages only. Challenge
			// broadcasts are notified from the upload handler, and system messages
			// (errors) are not user-facing.
			if incoming.notify && h.notify != nil && (message.Kind == "text" || message.Kind == "media") {
				h.notify(context.Background(), &message)
			}
		case <-h.stop:
			for client := range h.clients {
				// Teardown runs through the done channel: the send channel is
				// deliberately left open so a sendSystem call racing from a
				// readPump goroutine can never panic on a closed channel.
				close(client.done)
				delete(h.clients, client)
			}
			return
		}
	}
}

// remove deletes a client from every index and signals its write pump to
// stop. It runs on the single-threaded Run loop, so no locking is needed; the clients-map
// membership check makes double removal a no-op. The send channel is never
// closed: writePump terminates via the done channel instead, so a concurrent
// sendSystem call from a readPump goroutine can never panic on a closed
// channel.
func (h *Hub) remove(client *Client) {
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		if users := h.clientsByUser[client.userID]; users != nil {
			delete(users, client)
			if len(users) == 0 {
				delete(h.clientsByUser, client.userID)
			}
		}
		close(client.done)
	}
}

// DisconnectUser closes every live socket for a user across all of their
// groups. Credential revocation (logout-all, password change/reset, account
// deletion) invokes it so sockets opened under a now-revoked auth version
// close instead of receiving further events. The send is best-effort and
// non-blocking; the periodic revalidation is the backstop if a command is
// ever dropped.
func (h *Hub) DisconnectUser(userID string) {
	select {
	case h.disconnectUser <- userID:
	default:
	}
}

// DisconnectUserInGroup closes every live socket for a user within one group.
// A future membership-removal path uses it to revoke access to a single group
// without disturbing the user's sockets elsewhere.
func (h *Hub) DisconnectUserInGroup(userID, groupID string) {
	select {
	case h.disconnectUserGroup <- userGroupPair{userID: userID, groupID: groupID}:
	default:
	}
}

// disconnectSockets closes every socket matching the user, optionally
// restricted to one group. It runs on the single-threaded Run loop.
func (h *Hub) disconnectSockets(userID, groupID string) {
	for client := range h.clientsByUser[userID] {
		if groupID == "" || client.groupID == groupID {
			h.remove(client)
		}
	}
}

const revalidationWorkers = 8

// startRevalidation snapshots the current clients and validates them away
// from the hub event loop. Repository checks may take seconds; running them on
// the loop would stall broadcasts and explicit revocation disconnects. A
// bounded worker set avoids turning a large room into an unbounded goroutine
// burst. Results return to Run, which remains the sole owner of client state.
func (h *Hub) startRevalidation() bool {
	if h.Revalidate == nil || len(h.clients) == 0 {
		return false
	}
	clients := make([]*Client, 0, len(h.clients))
	for client := range h.clients {
		clients = append(clients, client)
	}
	go h.revalidateSnapshot(clients)
	return true
}

func (h *Hub) revalidateSnapshot(clients []*Client) {
	jobs := make(chan *Client)
	var workers sync.WaitGroup
	workerCount := min(revalidationWorkers, len(clients))
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for client := range jobs {
				result := revalidationResult{client: client, valid: h.revalidateClient(client)}
				select {
				case h.revalidationResults <- result:
				case <-h.stop:
					return
				}
			}
		}()
	}
	for _, client := range clients {
		select {
		case jobs <- client:
		case <-h.stop:
			close(jobs)
			workers.Wait()
			return
		}
	}
	close(jobs)
	workers.Wait()
	select {
	case h.revalidationDone <- struct{}{}:
	case <-h.stop:
	}
}

// RevalidateNow requests an immediate revalidation sweep. It is exposed for
// tests (and any future caller that needs out-of-band revalidation without
// waiting for the periodic tick). Repository checks run on bounded workers;
// their results are applied by the single-threaded Run loop.
func (h *Hub) RevalidateNow() {
	select {
	case h.revalidateNow <- struct{}{}:
	default:
	}
}

// revalidateClient reports whether a single client may keep exchanging
// messages. A nil revalidator always reports valid so hubs without one
// behave exactly as before F-03.
func (h *Hub) revalidateClient(client *Client) bool {
	if h.Revalidate == nil {
		return true
	}
	return h.Revalidate(client.userID, client.groupID)
}

// unregisterClient prevents a read pump from blocking forever when it exits
// after the hub has already stopped consuming unregister requests.
func (h *Hub) unregisterClient(client *Client) {
	select {
	case h.unregister <- client:
	case <-h.stop:
	}
}

func (h *Hub) Broadcast(message models.Message) { h.broadcast <- event{message: message, notify: true} }
func (h *Hub) BroadcastFrom(client *Client, message models.Message) {
	h.broadcast <- event{message: message, sender: client, notify: true}
}

// BroadcastPersisted delivers a message whose database transaction has already
// committed (for example, an HTTP multipart upload). It must never attempt a
// second persistence pass, but otherwise follows the ordinary broadcast and
// notification path.
func (h *Hub) BroadcastPersisted(message models.Message) {
	h.broadcast <- event{message: message, alreadyPersisted: true, notify: true}
}

// BroadcastUpdate delivers a persisted message update without treating it as
// a new chat message. Reactions use this path so updating a message cannot
// trigger another push notification.
func (h *Hub) BroadcastUpdate(message models.Message) {
	h.broadcast <- event{message: message, alreadyPersisted: true}
}

func (h *Hub) Stop() {
	h.once.Do(func() { close(h.stop) })
	select {
	case <-h.stopped:
	case <-time.After(5 * time.Second):
	}
}

// sendSystem queues a system message to a client without blocking. The send
// channel is never closed by the hub, so this select send can never panic on
// a closed channel even when it races with a disconnect; a full buffer simply
// drops the message (the client is already gone or slow).
func sendSystem(client *Client, code, content string) {
	select {
	case client.send <- models.Message{Kind: "system", Content: content, ErrorCode: code}:
	default:
	}
}

func newMessageID() string { return uuid.NewString() }
