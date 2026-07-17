package models

import "time"

type Message struct {
	ID        string    `json:"id"`
	GroupID   string    `json:"group_id"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Avatar    string    `json:"avatar"`
	Kind      string    `json:"kind"`
	PhotoID   *string   `json:"photo_id,omitempty"`
	ErrorCode string    `json:"error_code,omitempty"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
