package chat

import (
	"context"
	"geoguessme/internal/database"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	"log"
	"time"

	"github.com/google/uuid"
)

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan models.Message
	register   chan *Client
	unregister chan *Client
}

func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan models.Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			// Fetch username if not present (though it should be handled by the sender usually, but let's ensure it)
			// Actually, for simplicity, let's just save and broadcast. The frontend might need the username.
			// Fetch username and avatar from the database before broadcasting
			var username, avatar string
			query := `SELECT username, avatar FROM users WHERE id = $1`
			err := database.DB.QueryRow(context.Background(), query, message.UserID).Scan(&username, &avatar)
			if err != nil {
				log.Printf("Failed to fetch user details for message broadcast: %v", err)
				// Continue with default values or skip if user not found
			}

			// Save message to DB with generated ID and timestamp
			dbMsg := &models.Message{
				ID:        uuid.New().String(),
				GroupID:   message.GroupID,
				UserID:    message.UserID,
				Username:  username, // Use fetched username
				Avatar:    avatar,   // Use fetched avatar
				Content:   message.Content,
				CreatedAt: time.Now(),
			}
			if err := repository.SaveMessage(dbMsg); err != nil {
				log.Printf("Failed to save message: %v", err)
			}

			// Update the message to broadcast with the username, avatar, and ID
			msgToBroadcast := *dbMsg

			// Broadcast to connected clients
			for client := range h.clients {
				if client.groupID == message.GroupID {
					select {
					case client.send <- msgToBroadcast:
					default:
						close(client.send)
						delete(h.clients, client)
					}
				}
			}
		}
	}
}

func (h *Hub) Broadcast(msg models.Message) {
	h.broadcast <- msg
}
