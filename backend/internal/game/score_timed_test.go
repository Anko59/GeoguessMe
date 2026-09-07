package game

import (
	"testing"
	"time"
)

func TestTimeMultiplier(t *testing.T) {
	window := 5 * time.Minute
	if got := TimeMultiplier(30*time.Second, window); got != 1 {
		t.Fatalf("under 60s = %v, want 1", got)
	}
	if got := TimeMultiplier(60*time.Second, window); got != 1 {
		t.Fatalf("at 60s = %v, want 1", got)
	}
	if got := TimeMultiplier(299*time.Second, window); got != 0.2 {
		t.Fatalf("at 4m59s = %v, want 0.2", got)
	}
	if got := TimeMultiplier(5*time.Minute, window); got != 0 {
		t.Fatalf("at 5m = %v, want 0", got)
	}
	if got := TimeMultiplier(6*time.Minute, window); got != 0 {
		t.Fatalf("after window = %v, want 0", got)
	}
}

func TestCalculateScoreWithTime(t *testing.T) {
	window := 5 * time.Minute
	// Pinpoint: under 1m → 5000
	if got := CalculateScoreWithTime(0, 10*time.Second, window); got != 5000 {
		t.Fatalf("pinpoint fast = %d, want 5000", got)
	}
	// Pinpoint at 4m59s → 1000
	if got := CalculateScoreWithTime(0, 299*time.Second, window); got != 1000 {
		t.Fatalf("pinpoint at 4m59s = %d, want 1000", got)
	}
	// At 5m → 0
	if got := CalculateScoreWithTime(0, 5*time.Minute, window); got != 0 {
		t.Fatalf("at timeout = %d, want 0", got)
	}
}
