package models

import "time"

// ChatMedia is a private image or video attached to one group message. Storage
// details are deliberately not serialized: clients receive only an opaque id
// and retrieve the bytes through the authenticated media endpoint.
type ChatMedia struct {
	ID         string
	GroupID    string
	UserID     string
	StorageKey string
	MIMEType   string
	ByteSize   int64
	CreatedAt  time.Time
}
