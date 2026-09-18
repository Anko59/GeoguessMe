package models

import "time"

// GroupInbox is the server-owned summary used by the authenticated feed rail.
// UnreadCount is derived from persisted group messages and the viewer's read
// marker; clients must not infer it from a chat snapshot.
type GroupInbox struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	UnreadCount   int               `json:"unread_count"`
	LatestMessage *InboxMessageMeta `json:"latest_message,omitempty"`
}

type InboxMessageMeta struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}
