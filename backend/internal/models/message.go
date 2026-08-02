package models

import "time"

type Message struct {
	ID                string          `json:"id"`
	GroupID           string          `json:"group_id"`
	UserID            string          `json:"user_id"`
	Username          string          `json:"username"`
	Avatar            string          `json:"avatar"`
	Kind              string          `json:"kind"`
	PhotoID           *string         `json:"photo_id,omitempty"`
	MediaID           *string         `json:"media_id,omitempty"`
	MediaType         string          `json:"media_type,omitempty"`
	ReplyToID         *string         `json:"reply_to_id,omitempty"`
	ErrorCode         string          `json:"error_code,omitempty"`
	Content           string          `json:"content"`
	CreatedAt         time.Time       `json:"created_at"`
	ChallengeStatus   string          `json:"challenge_status,omitempty"`
	ChallengeResolved bool            `json:"challenge_resolved,omitempty"`
	Reactions         []Reaction      `json:"reactions"`
	ReactionUpdate    *ReactionUpdate `json:"reaction_update,omitempty"`
}

type Reaction struct {
	Reaction  string   `json:"reaction"`
	Emoji     string   `json:"emoji"`
	Count     int      `json:"count"`
	Reacted   bool     `json:"reacted"`
	Usernames []string `json:"usernames"`
}

// ReactionUpdate identifies the mutation behind a WebSocket aggregate update.
// Reaction aggregates contain viewer-specific Reacted flags, so clients use
// this delta to preserve their own selection while accepting the new counts.
type ReactionUpdate struct {
	UserID   string `json:"user_id"`
	Reaction string `json:"reaction"`
	Emoji    string `json:"emoji"`
	Active   bool   `json:"active"`
}
