// Package elo computes pairwise Elo ratings from shared-challenge guess
// results. A challenge is a photo that at least two players guessed; each
// pair of guessers on the same challenge is compared by raw score.
//
// Ratings are recomputed deterministically from guess history on every call,
// never stored: because a challenge's comparisons depend on who guessed it,
// a guess made on an old challenge retroactively changes the rating of every
// player who played it, and a later challenge's expected scores shift with
// them. This is what makes the rating measure guessing skill independently of
// who posts harder challenges.
package elo

import (
	"math"
	"sort"
	"time"
)

const (
	// InitialRating is every player's rating before their first comparison.
	InitialRating = 1000
	// K is the Elo update factor, the standard 32 for casual competition.
	K = 32
	// ratingScale is the classic 400-point Elo scale.
	ratingScale = 400.0
)

// Guess is one player's raw score on a challenge.
type Guess struct {
	UserID string
	Score  int
}

// Challenge is a photo with the guesses it received. Only challenges with at
// least two guessers produce comparisons.
type Challenge struct {
	ID        string
	CreatedAt time.Time
	Guesses   []Guess
}

func expectedScore(ratingA, ratingB int) float64 {
	return 1.0 / (1.0 + math.Pow(10, float64(ratingB-ratingA)/ratingScale))
}

// ComputeRatings returns the final rating of every player who compared
// against at least one other player on a shared challenge. Challenges are
// processed in chronological order (stable for equal timestamps); within a
// challenge every pair of guessers is compared and all rating deltas are
// applied together afterwards, so the order of a challenge's guessers does
// not matter.
func ComputeRatings(challenges []Challenge) map[string]int {
	sorted := make([]Challenge, len(challenges))
	copy(sorted, challenges)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
	})

	ratings := map[string]int{}
	deltas := map[string]float64{}
	for _, challenge := range sorted {
		guesses := challenge.Guesses
		if len(guesses) < 2 {
			continue
		}
		for i := 0; i < len(guesses); i++ {
			for j := i + 1; j < len(guesses); j++ {
				a, b := guesses[i], guesses[j]
				ra := rating(ratings, a.UserID)
				rb := rating(ratings, b.UserID)
				var resultA float64
				switch {
				case a.Score > b.Score:
					resultA = 1
				case a.Score < b.Score:
					resultA = 0
				default:
					resultA = 0.5
				}
				deltas[a.UserID] += K * (resultA - expectedScore(ra, rb))
				deltas[b.UserID] += K * ((1 - resultA) - expectedScore(rb, ra))
			}
		}
		for id, delta := range deltas {
			ratings[id] = rating(ratings, id) + int(math.Round(delta))
		}
		clear(deltas)
	}
	return ratings
}

// ComputeChallengeDeltas returns the Elo rating change for each participant on
// each challenge in the chronological replay. The returned map is keyed by
// challenge ID, then by user ID.
func ComputeChallengeDeltas(challenges []Challenge) map[string]map[string]int {
	sorted := make([]Challenge, len(challenges))
	copy(sorted, challenges)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
	})

	ratings := map[string]int{}
	deltas := map[string]float64{}
	challengeDeltas := make(map[string]map[string]int, len(sorted))
	for _, challenge := range sorted {
		guesses := challenge.Guesses
		if len(guesses) < 2 {
			continue
		}
		for i := 0; i < len(guesses); i++ {
			for j := i + 1; j < len(guesses); j++ {
				a, b := guesses[i], guesses[j]
				ra := rating(ratings, a.UserID)
				rb := rating(ratings, b.UserID)
				var resultA float64
				switch {
				case a.Score > b.Score:
					resultA = 1
				case a.Score < b.Score:
					resultA = 0
				default:
					resultA = 0.5
				}
				deltas[a.UserID] += K * (resultA - expectedScore(ra, rb))
				deltas[b.UserID] += K * ((1 - resultA) - expectedScore(rb, ra))
			}
		}
		userDeltas := make(map[string]int, len(deltas))
		for id, delta := range deltas {
			rounded := int(math.Round(delta))
			ratings[id] = rating(ratings, id) + rounded
			userDeltas[id] = rounded
		}
		challengeDeltas[challenge.ID] = userDeltas
		clear(deltas)
	}
	return challengeDeltas
}

func rating(ratings map[string]int, userID string) int {
	if rating, ok := ratings[userID]; ok {
		return rating
	}
	return InitialRating
}
