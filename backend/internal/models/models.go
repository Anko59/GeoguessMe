package models

import (
	"time"
)

type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"` // Hash
	Avatar    string    `json:"avatar"`
	Score     int       `json:"score"`
	CreatedAt time.Time `json:"created_at"`
}

type Group struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Code      string    `json:"code"` // Join code
	CreatedAt time.Time `json:"created_at"`
}

type GroupMember struct {
	GroupID  string    `json:"group_id"`
	UserID   string    `json:"user_id"`
	JoinedAt time.Time `json:"joined_at"`
}

type Photo struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	GroupID   string    `json:"group_id"`
	URL       string    `json:"url"`
	Lat       float64   `json:"-"` // Hidden from client until guessed? Or sent but hidden by frontend? Better hidden.
	Long      float64   `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"` // For ephemeral nature? Or just 10s view?
}

type Guess struct {
	ID        string    `json:"id"`
	PhotoID   string    `json:"photo_id"`
	UserID    string    `json:"user_id"`
	GroupID   string    `json:"group_id"`
	Lat       float64   `json:"lat"`
	Long      float64   `json:"long"`
	Score     int       `json:"score"`
	Distance  float64   `json:"distance"` // in meters
	CreatedAt time.Time `json:"created_at"`
}
