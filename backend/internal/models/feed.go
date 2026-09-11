package models

import "time"

// PublicChallenge contains only feed-safe fields. Coordinates and storage keys
// are deliberately absent; only the guess result can reveal the answer.
type PublicChallenge struct {
	ID            string    `json:"id"`
	UserID        string    `json:"user_id"`
	Username      string    `json:"username"`
	Caption       string    `json:"caption"`
	CreatedAt     time.Time `json:"created_at"`
	IsOwner       bool      `json:"is_owner"`
	Resolved      bool      `json:"resolved"`
	ReactionCount int       `json:"reaction_count"`
	Reacted       bool      `json:"reacted"`
	CommentCount  int       `json:"comment_count"`
}

type PublicFeedPage struct {
	Items      []PublicChallenge `json:"items"`
	NextCursor string            `json:"next_cursor"`
}

type PublicComment struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	CanDelete bool      `json:"can_delete"`
}

type PublicCommentsPage struct {
	Items      []PublicComment `json:"items"`
	NextCursor string          `json:"next_cursor"`
}

type PublicGuessResult struct {
	Score      int     `json:"score"`
	Distance   float64 `json:"distance"`
	Lat        float64 `json:"lat"`
	Long       float64 `json:"long"`
	ActualLat  float64 `json:"actual_lat"`
	ActualLong float64 `json:"actual_long"`
}
