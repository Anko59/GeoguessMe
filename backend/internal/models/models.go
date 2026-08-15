package models

import (
	"time"
)

type User struct {
	ID              string     `json:"id"`
	Username        string     `json:"username"`
	Email           string     `json:"email,omitempty"` // verified contact; empty when unverified (column NULL)
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	PendingEmail    string     `json:"-"` // submitted-but-unverified contact claim
	Password        string     `json:"-"`
	Avatar          string     `json:"avatar"`
	AuthVersion     int        `json:"-"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Group struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Code      string    `json:"-"` // Legacy join code: retained in the database, never on the wire (F-06)
	CreatedAt time.Time `json:"created_at"`
}

// GroupInvite is a revocable, expiring bearer invite token stored only as a
// SHA-256 hash. The raw token and invite URL are returned exactly once at
// creation; list responses and every other wire shape omit them entirely.
type GroupInvite struct {
	ID            string     `json:"id"`
	GroupID       string     `json:"group_id"`
	CreatorUserID string     `json:"creator_user_id"`
	TokenHash     string     `json:"-"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	RevokedAt     *time.Time `json:"revoked"`
}

type GroupPhoto struct {
	GroupID    string    `json:"group_id"`
	StorageKey string    `json:"-"`
	MIMEType   string    `json:"mime_type"`
	ByteSize   int64     `json:"byte_size"`
	CreatedAt  time.Time `json:"created_at"`
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
	HideLocation    bool      `json:"hide_location"`
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
