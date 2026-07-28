package models

import "time"

type Message struct {
	ID                string     `json:"id"`
	GroupID           string     `json:"group_id"`
	UserID            string     `json:"user_id"`
	Username          string     `json:"username"`
	Avatar            string     `json:"avatar"`
	Kind              string     `json:"kind"`
	PhotoID           *string    `json:"photo_id,omitempty"`
	MediaID           *string    `json:"media_id,omitempty"`
	MediaType         string     `json:"media_type,omitempty"`
	ReplyToID         *string    `json:"reply_to_id,omitempty"`
	ErrorCode         string     `json:"error_code,omitempty"`
	Content           string     `json:"content"`
	CreatedAt         time.Time  `json:"created_at"`
	ChallengeStatus   string     `json:"challenge_status,omitempty"`
	ChallengeResolved bool       `json:"challenge_resolved,omitempty"`
	Reactions         []Reaction `json:"reactions,omitempty"`
}

type Reaction struct {
	Emoji   string `json:"emoji"`
	Count   int    `json:"count"`
	Reacted bool   `json:"reacted"`
}
