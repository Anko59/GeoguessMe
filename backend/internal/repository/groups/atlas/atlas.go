// Package atlas reads the complete group challenge history for the globe.
package atlas

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"geoguessme/internal/database"
	"geoguessme/internal/game"
	"geoguessme/internal/models"

	"github.com/google/uuid"
)

// ErrInvalidCursor identifies a malformed cursor or one from another group.
var ErrInvalidCursor = errors.New("invalid challenge cursor")

// Challenge contains only information the viewer is allowed to see.
type Challenge struct {
	PhotoID           string     `json:"photo_id"`
	GroupID           string     `json:"group_id"`
	UserID            string     `json:"user_id"`
	Username          string     `json:"username"`
	CreatedAt         time.Time  `json:"created_at"`
	ExpiresAt         time.Time  `json:"expires_at"`
	Status            string     `json:"status"`
	Lat               *float64   `json:"lat,omitempty"`
	Long              *float64   `json:"long,omitempty"`
	LocationRevealsAt *time.Time `json:"location_reveals_at,omitempty"`
}

// Page is ordered newest first, including challenges whose media was removed.
type Page struct {
	Items      []Challenge `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty"`
	ServerTime time.Time   `json:"server_time"`
}

func decodeCursor(raw, groupID string) (time.Time, string, error) {
	if raw == "" {
		return time.Time{}, "", nil
	}
	if len(raw) > 512 {
		return time.Time{}, "", ErrInvalidCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, "", ErrInvalidCursor
	}
	parts := strings.Split(string(data), "|")
	if len(parts) != 3 || parts[0] != groupID {
		return time.Time{}, "", ErrInvalidCursor
	}
	at, err := time.Parse(time.RFC3339Nano, parts[1])
	if err != nil || at.IsZero() {
		return time.Time{}, "", ErrInvalidCursor
	}
	if _, err := uuid.Parse(parts[2]); err != nil {
		return time.Time{}, "", ErrInvalidCursor
	}
	return at, parts[2], nil
}

// List uses keyset pagination independent of chat history. Membership is also
// checked in the data query so a revoked member cannot read a subsequent page.
func List(ctx context.Context, pool database.Pool, groupID, viewerID, cursor string, now time.Time, hideDuration time.Duration) (Page, error) {
	at, id, err := decodeCursor(cursor, groupID)
	if err != nil {
		return Page{}, err
	}
	const pageSize = 100
	query := `SELECT p.id, p.group_id, p.user_id, u.username, p.created_at, p.expires_at,
		p.lat, p.long, p.hide_location,
		EXISTS (SELECT 1 FROM guesses g WHERE g.photo_id = p.id AND g.user_id = $2)
		FROM photos p JOIN users u ON u.id = p.user_id
		WHERE p.group_id = $1
		AND EXISTS (SELECT 1 FROM group_members m WHERE m.group_id = p.group_id AND m.user_id = $2)`
	args := []any{groupID, viewerID}
	if id != "" {
		query += ` AND (p.created_at, p.id) < ($3, $4)`
		args = append(args, at, id)
	}
	query += ` ORDER BY p.created_at DESC, p.id DESC LIMIT 101`
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	page := Page{Items: make([]Challenge, 0, pageSize), ServerTime: now}
	for rows.Next() {
		var photo models.Photo
		var username string
		var guessed bool
		if err := rows.Scan(&photo.ID, &photo.GroupID, &photo.UserID, &username, &photo.CreatedAt, &photo.ExpiresAt, &photo.Lat, &photo.Long, &photo.HideLocation, &guessed); err != nil {
			return Page{}, err
		}
		if len(page.Items) == pageSize {
			last := page.Items[pageSize-1]
			page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(groupID + "|" + last.CreatedAt.Format(time.RFC3339Nano) + "|" + last.PhotoID))
			break
		}
		page.Items = append(page.Items, visibleChallenge(&photo, username, viewerID, guessed, now, hideDuration))
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	return page, nil
}

func visibleChallenge(photo *models.Photo, username, viewerID string, guessed bool, now time.Time, hideDuration time.Duration) Challenge {
	item := Challenge{PhotoID: photo.ID, GroupID: photo.GroupID, UserID: photo.UserID, Username: username, CreatedAt: photo.CreatedAt, ExpiresAt: photo.ExpiresAt, Status: "available"}
	switch {
	case photo.UserID == viewerID:
		item.Status = "results"
	case guessed:
		item.Status = "guessed"
	case !now.Before(photo.ExpiresAt):
		item.Status = "expired"
	}
	if game.LocationHidden(photo, viewerID, now, hideDuration) {
		revealsAt := photo.CreatedAt.Add(hideDuration)
		item.LocationRevealsAt = &revealsAt
	} else if item.Status != "available" {
		item.Lat, item.Long = &photo.Lat, &photo.Long
	}
	return item
}
