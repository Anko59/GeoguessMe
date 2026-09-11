package feed

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidCursor = errors.New("invalid cursor")

type Cursor struct {
	CreatedAt time.Time `json:"at"`
	ID        string    `json:"id"`
}

func ParseCursor(value string) (Cursor, error) {
	var cursor Cursor
	if value == "" {
		return cursor, nil
	}
	if len(value) > 256 {
		return cursor, ErrInvalidCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return cursor, ErrInvalidCursor
	}
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.CreatedAt.IsZero() {
		return Cursor{}, ErrInvalidCursor
	}
	if _, err := uuid.Parse(cursor.ID); err != nil {
		return Cursor{}, ErrInvalidCursor
	}
	return cursor, nil
}

func encodeCursor(at time.Time, id string) string {
	// Both inputs have fixed JSON encodings and cannot fail to marshal.
	data, _ := json.Marshal(Cursor{CreatedAt: at, ID: id})
	return base64.RawURLEncoding.EncodeToString(data)
}
