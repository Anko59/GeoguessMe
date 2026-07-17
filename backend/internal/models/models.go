package models

import (
	"time"
)

type User struct {
	ID              string     `json:"id"`
	Username        string     `json:"username"`
	Email           string     `json:"email"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	Password        string     `json:"-"`
	Avatar          string     `json:"avatar"`
	AuthVersion     int        `json:"-"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
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
	ID              string    `json:"id"`
	UserID          string    `json:"user_id"`
	GroupID         string    `json:"group_id"`
	URL             string    `json:"-"`
	StorageKey      string    `json:"-"`
	MIMEType        string    `json:"mime_type"`
	ByteSize        int64     `json:"byte_size"`
	Lat             float64   `json:"-"`
	Long            float64   `json:"-"`
	LifecycleStatus string    `json:"lifecycle_status"`
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	RetentionAt     time.Time `json:"-"`
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
