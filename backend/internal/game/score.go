package game

import (
	"math"
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
