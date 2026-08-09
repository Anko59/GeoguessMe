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

type Hub struct {
	clients        map[*Client]bool
	broadcast      chan event
	register       chan *Client
	unregister     chan *Client
	stop           chan struct{}
	stopped        chan struct{}
	persist        PersistFunc
	notify         NotifyFunc
	persistTimeout time.Duration
	once           sync.Once
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

// NewHubWithTimeout builds a hub with an explicit persistence deadline. It is
// the constructor tests use to exercise the bounded-persist invariant with a
// tiny timeout instead of waiting five seconds. A non-positive timeout falls
// back to the default so a hub can never be accidentally unbounded.
func NewHubWithTimeout(persist PersistFunc, notify NotifyFunc, persistTimeout time.Duration) *Hub {
	if persistTimeout <= 0 {
		persistTimeout = defaultPersistTimeout
	}
	return &Hub{broadcast: make(chan event, 128), register: make(chan *Client), unregister: make(chan *Client), clients: make(map[*Client]bool), stop: make(chan struct{}), stopped: make(chan struct{}), persist: persist, notify: notify, persistTimeout: persistTimeout}
}

func (h *Hub) Run() {
	defer close(h.stopped)
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
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
				close(client.send)
				delete(h.clients, client)
			}
			return
		}
	}
}

func (h *Hub) remove(client *Client) {
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
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

func sendSystem(client *Client, code, content string) {
	select {
	case client.send <- models.Message{Kind: "system", Content: content, ErrorCode: code}:
	default:
	}
}

func newMessageID() string { return uuid.NewString() }
