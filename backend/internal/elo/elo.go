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
//
// Every ladder replays history the same way but moves at its own speed: the
// weekly ladder uses the largest update factor so it rewards current form,
// the monthly ladder a moderate one, and the all-time ladder the smallest so
// its rating converges toward durable skill instead of tracking the last few
// challenges.
package elo

import (
	"math"
	"sort"
	"time"
)

const (
	// InitialRating is every player's rating before their first comparison.
	InitialRating = 1000
	// ratingScale is the classic 400-point Elo scale.
	ratingScale = 400.0
)

// Factor is the Elo update factor of a rating ladder: the maximum points two
// equally rated players can exchange on one comparison. Larger factors make a
// ladder react faster to recent results; smaller ones make it more stable.
type Factor float64

const (
	// FactorWeekly drives weekly ladders, which reset often and should
	// reflect this week's form.
	FactorWeekly Factor = 40
	// FactorMonthly drives monthly ladders, balancing responsiveness
	// against week-to-week noise.
	FactorMonthly Factor = 20
	// FactorAllTime drives all-time ladders and the global profile rating,
	// which must move slowly enough to measure long-run skill rather than
	// the most recent challenges.
	FactorAllTime Factor = 8
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

// replay processes the challenges in chronological order (stable for equal
// timestamps). Within a challenge every pair of guessers is compared by raw
// score and all rating deltas are applied together afterwards, so the order
// of a challenge's guessers does not matter. It returns every rated player's
// final rating plus each challenge's per-user rounded rating change.
func replay(challenges []Challenge, factor Factor) (map[string]int, map[string]map[string]int) {
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
				deltas[a.UserID] += float64(factor) * (resultA - expectedScore(ra, rb))
				deltas[b.UserID] += float64(factor) * ((1 - resultA) - expectedScore(rb, ra))
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
	return ratings, challengeDeltas
}

// ComputeRatings returns the final rating of every player who compared
// against at least one other player on a shared challenge, moving ratings at
// the given ladder factor.
func ComputeRatings(challenges []Challenge, factor Factor) map[string]int {
	ratings, _ := replay(challenges, factor)
	return ratings
}

// ComputeChallengeDeltas returns the Elo rating change for each participant on
// each challenge in the chronological replay, using the given ladder factor.
// The returned map is keyed by challenge ID, then by user ID.
func ComputeChallengeDeltas(challenges []Challenge, factor Factor) map[string]map[string]int {
	_, deltas := replay(challenges, factor)
	return deltas
}

func rating(ratings map[string]int, userID string) int {
	if rating, ok := ratings[userID]; ok {
		return rating
	}
	return InitialRating
}
