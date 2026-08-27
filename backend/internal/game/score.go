package game

import (
	"math"
	"time"
)

// CalculateDistance returns the distance between two coordinates in meters using Haversine formula
func CalculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Earth radius in meters

	dLat := (lat2 - lat1) * (math.Pi / 180.0)
	dLon := (lon2 - lon1) * (math.Pi / 180.0)

	lat1Rad := lat1 * (math.Pi / 180.0)
	lat2Rad := lat2 * (math.Pi / 180.0)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(lat1Rad)*math.Cos(lat2Rad)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

// CalculateScore returns a score between 0 and 5000 based on distance
func CalculateScore(distance float64) int {
	if distance < 50 {
		return 5000
	}
	// Exponential decay: 5000 * e^(-distance / 20000)
	// Adjust 20000 to change difficulty.
	score := 5000 * math.Exp(-distance/20000)
	return int(math.Round(score))
}

// TimeMultiplier returns the time penalty factor in [0, 1] for the guessing
// window. Under 60s the score is unchanged (1.0). Afterwards the achievable
// score decays linearly so a pinpoint guess at 4m59s (one second before the
// 5-minute window expires) is worth 1000 points (0.2×), and at or after the
// window the factor is 0. The window is the configured GUESS_WINDOW
// (default 5m); the formula generalizes for other values.
func TimeMultiplier(elapsed, guessWindow time.Duration) float64 {
	if guessWindow <= 0 {
		return 1
	}
	if elapsed < 60*time.Second {
		return 1
	}
	if elapsed >= guessWindow {
		return 0
	}
	// Linear decay from 1.0 at 60s to 0.2 at (guessWindow - 1s).
	penaltySpan := guessWindow - 60*time.Second - time.Second
	if penaltySpan <= 0 {
		return 1
	}
	elapsedOffset := elapsed - 60*time.Second
	if elapsedOffset >= penaltySpan {
		return 0.2
	}
	if elapsedOffset <= 0 {
		return 1
	}
	ratio := float64(elapsedOffset) / float64(penaltySpan)
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	mult := 1 - 0.8*ratio
	// Snap the 0.2 anchor to an exact representable value so callers can
	// compare with == 0.2 without tripping on 0.1999… from binary 0.8.
	if mult < 0.2000005 && mult > 0.1999995 {
		return 0.2
	}
	return mult
}

// CalculateScoreWithTime returns the distance-based score scaled by the time
// penalty. A nil guessWindow is treated as the default 5m.
func CalculateScoreWithTime(distance float64, elapsed, guessWindow time.Duration) int {
	base := CalculateScore(distance)
	if guessWindow <= 0 {
		guessWindow = 5 * time.Minute
	}
	mult := TimeMultiplier(elapsed, guessWindow)
	return int(math.Round(float64(base) * mult))
}
