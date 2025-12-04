package game

import (
	"testing"
)

func TestCalculateDistance(t *testing.T) {
	// London to Paris
	lat1, lon1 := 51.5074, -0.1278
	lat2, lon2 := 48.8566, 2.3522
	expected := 344000.0 // Approx 344km

	dist := CalculateDistance(lat1, lon1, lat2, lon2)

	// Allow 1% margin of error
	if dist < expected*0.99 || dist > expected*1.01 {
		t.Errorf("Expected distance around %v, got %v", expected, dist)
	}
}

func TestCalculateScore(t *testing.T) {
	tests := []struct {
		distance float64
		minScore int
		maxScore int
	}{
		{0, 5000, 5000},    // Perfect guess
		{40, 5000, 5000},   // Close enough
		{2000, 1800, 1900}, // ~1/e decay
		{20000000, 0, 1},   // Other side of world
	}

	for _, tt := range tests {
		score := CalculateScore(tt.distance)
		if score < tt.minScore || score > tt.maxScore {
			t.Errorf("Distance %v: expected score between %v and %v, got %v", tt.distance, tt.minScore, tt.maxScore, score)
		}
	}
}
