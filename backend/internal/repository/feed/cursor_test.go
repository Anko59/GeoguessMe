package feed

import (
	"testing"
	"time"
)

func TestCursor(t *testing.T) {
	now := time.Date(2026, 9, 12, 10, 0, 0, 123000, time.UTC)
	id := "00000000-0000-0000-0000-000000000001"
	c, err := ParseCursor(encodeCursor(now, id))
	if err != nil || c.ID != id || !c.CreatedAt.Equal(now) {
		t.Fatalf("round trip: %+v %v", c, err)
	}
	for _, value := range []string{"garbage", "e30", "bnVsbA"} {
		if _, err := ParseCursor(value); err == nil {
			t.Fatalf("accepted invalid cursor %q", value)
		}
	}
	if _, err := ParseCursor(""); err != nil {
		t.Fatal(err)
	}
}
