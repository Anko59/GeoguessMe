package elo

import (
	"testing"
	"time"
)

func challengeTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func TestComputeRatingsBasicWinLose(t *testing.T) {
	ratings := ComputeRatings([]Challenge{
		{
			ID:        "c1",
			CreatedAt: challengeTime("2026-01-01T00:00:00Z"),
			Guesses:   []Guess{{UserID: "alice", Score: 4500}, {UserID: "bob", Score: 500}},
		},
	})
	if len(ratings) != 2 {
		t.Fatalf("ratings = %v, want both players", ratings)
	}
	if ratings["alice"] <= InitialRating {
		t.Fatalf("winner rating = %d, want above %d", ratings["alice"], InitialRating)
	}
	if ratings["bob"] >= InitialRating {
		t.Fatalf("loser rating = %d, want below %d", ratings["bob"], InitialRating)
	}
	// The win and loss must be symmetric around the initial rating.
	if got := ratings["alice"] + ratings["bob"]; got != 2*InitialRating {
		t.Fatalf("ratings sum = %d, want %d (symmetric exchange)", got, 2*InitialRating)
	}
}

func TestComputeRatingsTie(t *testing.T) {
	ratings := ComputeRatings([]Challenge{
		{
			ID:        "c1",
			CreatedAt: challengeTime("2026-01-01T00:00:00Z"),
			Guesses:   []Guess{{UserID: "alice", Score: 3000}, {UserID: "bob", Score: 3000}},
		},
	})
	if ratings["alice"] != InitialRating || ratings["bob"] != InitialRating {
		t.Fatalf("tied ratings = %v, want both at %d", ratings, InitialRating)
	}
}

func TestComputeRatingsSingleGuesserUnrated(t *testing.T) {
	ratings := ComputeRatings([]Challenge{
		{
			ID:        "c1",
			CreatedAt: challengeTime("2026-01-01T00:00:00Z"),
			Guesses:   []Guess{{UserID: "solo", Score: 4000}},
		},
	})
	if _, ok := ratings["solo"]; ok {
		t.Fatalf("solo guesser must not be rated, got %v", ratings)
	}
}

func TestComputeRatingsWithinChallengeOrderDoesNotMatter(t *testing.T) {
	base := []Challenge{
		{ID: "c1", CreatedAt: challengeTime("2026-01-01T00:00:00Z"), Guesses: []Guess{{UserID: "alice", Score: 1000}, {UserID: "bob", Score: 4000}, {UserID: "carol", Score: 2500}}},
	}
	backward := []Challenge{
		{ID: "c1", CreatedAt: challengeTime("2026-01-01T00:00:00Z"), Guesses: []Guess{{UserID: "carol", Score: 2500}, {UserID: "bob", Score: 4000}, {UserID: "alice", Score: 1000}}},
	}
	r1 := ComputeRatings(base)
	r2 := ComputeRatings(backward)
	for _, user := range []string{"alice", "bob", "carol"} {
		if r1[user] != r2[user] {
			t.Fatalf("order-dependent rating for %s: %d vs %d", user, r1[user], r2[user])
		}
	}
}

func TestComputeRatingsLateGuessChangesPriorRatings(t *testing.T) {
	// A new guess on the first challenge must retroactively change the
	// ratings produced by that challenge.
	before := ComputeRatings([]Challenge{
		{ID: "c1", CreatedAt: challengeTime("2026-01-01T00:00:00Z"), Guesses: []Guess{{UserID: "alice", Score: 4500}, {UserID: "bob", Score: 500}}},
	})
	after := ComputeRatings([]Challenge{
		{ID: "c1", CreatedAt: challengeTime("2026-01-01T00:00:00Z"), Guesses: []Guess{{UserID: "alice", Score: 4500}, {UserID: "bob", Score: 500}, {UserID: "carol", Score: 4800}}},
	})
	if before["alice"] == after["alice"] {
		t.Fatalf("late guess must change alice's rating: %d == %d", before["alice"], after["alice"])
	}
	if before["bob"] == after["bob"] {
		t.Fatalf("late guess must change bob's rating: %d == %d", before["bob"], after["bob"])
	}
	if _, ok := after["carol"]; !ok {
		t.Fatalf("late guesser must be rated, got %v", after)
	}
}

func TestComputeRatingsSecondWinIsSmaller(t *testing.T) {
	// Beating the same player twice must gain less than twice a single win,
	// because the first win raises the rating the second win is measured from.
	firstWin := float64(K) * (1 - expectedScore(InitialRating, InitialRating))
	ratings := ComputeRatings([]Challenge{
		{ID: "c1", CreatedAt: challengeTime("2026-01-01T00:00:00Z"), Guesses: []Guess{{UserID: "alice", Score: 5000}, {UserID: "bob", Score: 0}}},
		{ID: "c2", CreatedAt: challengeTime("2026-01-02T00:00:00Z"), Guesses: []Guess{{UserID: "alice", Score: 5000}, {UserID: "bob", Score: 0}}},
	})
	if got := float64(ratings["alice"] - InitialRating); got >= 2*firstWin {
		t.Fatalf("two wins = %v, want less than %v (diminishing returns)", got, 2*firstWin)
	}
}
