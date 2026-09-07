package elo

import (
	"strconv"
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
	}, FactorAllTime)
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

// TestFirstWinScalesWithFactor pins the per-ladder update speeds: two equally
// rated players exchange exactly factor/2 points on a decisive comparison.
func TestFirstWinScalesWithFactor(t *testing.T) {
	challenges := []Challenge{
		{ID: "c1", CreatedAt: challengeTime("2026-01-01T00:00:00Z"), Guesses: []Guess{{UserID: "alice", Score: 4500}, {UserID: "bob", Score: 500}}},
	}
	for factor, wantGain := range map[Factor]int{FactorWeekly: 20, FactorMonthly: 10, FactorAllTime: 4} {
		ratings := ComputeRatings(challenges, factor)
		if ratings["alice"] != InitialRating+wantGain || ratings["bob"] != InitialRating-wantGain {
			t.Fatalf("factor %v gains = %+v, want +%d/-%d around %d", factor, ratings, wantGain, wantGain, InitialRating)
		}
	}
}

// TestLadderFactorsOrderVolatility is the regression test for rating
// volatility: the identical history must move the weekly ladder the most and
// the all-time ladder the least, so all-time reflects durable skill rather
// than recent results.
func TestLadderFactorsOrderVolatility(t *testing.T) {
	var challenges []Challenge
	for day := 1; day <= 5; day++ {
		challenges = append(challenges, Challenge{
			ID:        "c" + strconv.Itoa(day),
			CreatedAt: challengeTime("2026-01-0" + strconv.Itoa(day) + "T00:00:00Z"),
			Guesses:   []Guess{{UserID: "alice", Score: 4500}, {UserID: "bob", Score: 500}},
		})
	}
	gain := func(factor Factor) int {
		return ComputeRatings(challenges, factor)["alice"] - InitialRating
	}
	weekly, monthly, allTime := gain(FactorWeekly), gain(FactorMonthly), gain(FactorAllTime)
	if !(weekly > monthly && monthly > allTime && allTime > 0) {
		t.Fatalf("five identical wins moved weekly=%d, monthly=%d, all-time=%d; want weekly > monthly > all-time > 0", weekly, monthly, allTime)
	}
}

func TestComputeRatingsTie(t *testing.T) {
	ratings := ComputeRatings([]Challenge{
		{
			ID:        "c1",
			CreatedAt: challengeTime("2026-01-01T00:00:00Z"),
			Guesses:   []Guess{{UserID: "alice", Score: 3000}, {UserID: "bob", Score: 3000}},
		},
	}, FactorAllTime)
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
	}, FactorAllTime)
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
	r1 := ComputeRatings(base, FactorAllTime)
	r2 := ComputeRatings(backward, FactorAllTime)
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
	}, FactorAllTime)
	after := ComputeRatings([]Challenge{
		{ID: "c1", CreatedAt: challengeTime("2026-01-01T00:00:00Z"), Guesses: []Guess{{UserID: "alice", Score: 4500}, {UserID: "bob", Score: 500}, {UserID: "carol", Score: 4800}}},
	}, FactorAllTime)
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
	firstWin := float64(FactorWeekly) * (1 - expectedScore(InitialRating, InitialRating))
	ratings := ComputeRatings([]Challenge{
		{ID: "c1", CreatedAt: challengeTime("2026-01-01T00:00:00Z"), Guesses: []Guess{{UserID: "alice", Score: 5000}, {UserID: "bob", Score: 0}}},
		{ID: "c2", CreatedAt: challengeTime("2026-01-02T00:00:00Z"), Guesses: []Guess{{UserID: "alice", Score: 5000}, {UserID: "bob", Score: 0}}},
	}, FactorWeekly)
	if got := float64(ratings["alice"] - InitialRating); got >= 2*firstWin {
		t.Fatalf("two wins = %v, want less than %v (diminishing returns)", got, 2*firstWin)
	}
}

func TestComputeChallengeDeltas(t *testing.T) {
	challenges := []Challenge{
		{
			ID:        "c1",
			CreatedAt: challengeTime("2026-01-01T00:00:00Z"),
			Guesses:   []Guess{{UserID: "alice", Score: 4500}, {UserID: "bob", Score: 500}},
		},
		{
			ID:        "c2",
			CreatedAt: challengeTime("2026-01-02T00:00:00Z"),
			Guesses:   []Guess{{UserID: "alice", Score: 3000}, {UserID: "bob", Score: 3000}},
		},
		{
			ID:        "c3",
			CreatedAt: challengeTime("2026-01-03T00:00:00Z"),
			Guesses:   []Guess{{UserID: "solo", Score: 4000}},
		},
	}
	deltas := ComputeChallengeDeltas(challenges, FactorWeekly)

	// c1: Alice won against Bob (both starting at 1000).
	if deltas["c1"]["alice"] != 20 || deltas["c1"]["bob"] != -20 {
		t.Fatalf("c1 deltas = %v, want alice: 20, bob: -20", deltas["c1"])
	}

	// c2: Alice and Bob tied. Alice was higher rated (1020 vs 980), so tying loses a small amount of Elo for Alice and gains for Bob.
	if deltas["c2"]["alice"] >= 0 || deltas["c2"]["bob"] <= 0 {
		t.Fatalf("c2 deltas = %v, higher-rated player tying must lose elo", deltas["c2"])
	}
	if deltas["c2"]["alice"]+deltas["c2"]["bob"] != 0 {
		t.Fatalf("c2 sum = %d, want 0", deltas["c2"]["alice"]+deltas["c2"]["bob"])
	}

	// c3: Single guesser has no comparisons.
	if len(deltas["c3"]) != 0 {
		t.Fatalf("c3 deltas = %v, want empty", deltas["c3"])
	}
}

func TestComputeChallengeDeltasMatchesRatings(t *testing.T) {
	challenges := []Challenge{
		{ID: "c1", CreatedAt: challengeTime("2026-01-01T00:00:00Z"), Guesses: []Guess{{UserID: "alice", Score: 4500}, {UserID: "bob", Score: 500}}},
		{ID: "c2", CreatedAt: challengeTime("2026-01-02T00:00:00Z"), Guesses: []Guess{{UserID: "alice", Score: 1000}, {UserID: "bob", Score: 4000}}},
	}
	deltas := ComputeChallengeDeltas(challenges, FactorMonthly)
	ratings := ComputeRatings(challenges, FactorMonthly)
	if got := deltas["c1"]["alice"] + deltas["c2"]["alice"]; got != ratings["alice"]-InitialRating {
		t.Fatalf("delta sum %d does not rebuild alice's rating %d", got, ratings["alice"]-InitialRating)
	}
	if got := deltas["c1"]["alice"] + deltas["c1"]["bob"]; got != 0 {
		t.Fatalf("per-challenge deltas must be zero-sum, got %d", got)
	}
}
