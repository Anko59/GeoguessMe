package repository

import (
	"testing"
	"time"
)

func TestMessageCursorRoundTrip(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 123456789, time.UTC)
	const id = "f1e2d3c4-0000-0000-0000-000000000001"
	cursor := encodeMessageCursor(now, id)
	gotAt, gotID, err := decodeMessageCursor(cursor)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if gotID != id || !gotAt.Equal(now) {
		t.Fatalf("round trip mismatch: got %s @ %v want %s @ %v", gotID, gotAt, id, now)
	}
}

func TestMessageCursorRejectsMalformed(t *testing.T) {
	for _, bad := range []string{"", "not-base64!!", "++++", "onlyonepart", "abc|"} {
		if _, _, err := decodeMessageCursor(bad); err == nil {
			t.Errorf("expected error for cursor %q", bad)
		}
	}
}
