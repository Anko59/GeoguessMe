// Package chat owns the chat slice's PostgreSQL persistence. It is the
// dependency-injected persistence layer behind the handlers.ChatAPI: message
// persistence and pagination, viewer enrichment, reactions, chat media, and
// WebSocket tickets all live here as methods on Repository bound to an
// injected pool.
//
// The package is split by responsibility — messages, enrichment, reactions,
// media, and tickets — while scan.go stays the single owner of the shared
// message row shape and scanning. PR 5 moved these operations out of the
// flat internal/repository package (which is at its structural file limit) and
// off the database.DB global.
package chat

import (
	"errors"
	"time"

	"geoguessme/internal/database"
	"geoguessme/internal/models"

	"github.com/google/uuid"
)

var (
	// ErrInvalidMessageReply reports a reply_to_id that does not reference an
	// existing message in the same group.
	ErrInvalidMessageReply = errors.New("invalid message reply")
	// ErrInvalidReaction reports an empty reaction key.
	ErrInvalidReaction = errors.New("invalid reaction")
)

// Repository is the chat slice's persistence collection, bound to an injected
// pool. Instances are independent: two Repositories built on different pools
// never share state. The concrete *repository.Repository (parent package)
// holds the chat Repository as its Chat field, so the application composition
// root constructs it once alongside the other persistence slices.
type Repository struct {
	pool database.Pool
}

// NewRepository returns a chat Repository bound to the given pool.
func NewRepository(pool database.Pool) *Repository {
	return &Repository{pool: pool}
}

// MessagesPage is the cursor-paginated result of GetGroupMessagesPage.
//
// StableCursor is the opaque cursor positioned strictly after the last message
// of the page (empty for an empty page). Clients snapshot it before a chat
// reconnect and send it back as the cursor parameter so catch-up resumes
// exactly where the previous page ended; it replaces the legacy after_id
// message-id bridge removed by the compatibility-removal PR.
type MessagesPage struct {
	Items        []models.Message `json:"items"`
	NextCursor   string           `json:"next_cursor"`
	StableCursor string           `json:"stable_cursor"`
}

// NewTextMessage ensures the explicit timestamp is always initialized by
// server code.
func NewTextMessage(groupID, userID, content string, now time.Time) *models.Message {
	return &models.Message{ID: uuid.NewString(), GroupID: groupID, UserID: userID, Kind: "text", Content: content, CreatedAt: now}
}
