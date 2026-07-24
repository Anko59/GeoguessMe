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
	message models.Message
	sender  *Client
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan event
	register   chan *Client
	unregister chan *Client
	stop       chan struct{}
	stopped    chan struct{}
	persist    PersistFunc
	notify     NotifyFunc
	once       sync.Once
}

func NewHub(persist PersistFunc, notify NotifyFunc) *Hub {
	return &Hub{broadcast: make(chan event, 128), register: make(chan *Client), unregister: make(chan *Client), clients: make(map[*Client]bool), stop: make(chan struct{}), stopped: make(chan struct{}), persist: persist, notify: notify}
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
			if h.persist != nil {
				if err := h.persist(context.Background(), &message); err != nil {
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
			if h.notify != nil && message.Kind == "text" {
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

func (h *Hub) Broadcast(message models.Message) { h.broadcast <- event{message: message} }
func (h *Hub) BroadcastFrom(client *Client, message models.Message) {
	h.broadcast <- event{message: message, sender: client}
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
