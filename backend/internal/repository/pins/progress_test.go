package pins

import (
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v5"
)

func TestPerfectGuessCountsPartyTimeDoubledScore(t *testing.T) {
	repo, pool := mockPinsRepository(t)
	pool.ExpectQuery("SELECT COUNT").WithArgs("user", 5000, 1).WillReturnRows(
		pgxmock.NewRows([]string{"eligible"}).AddRow(true),
	)
	eligible, err := repo.challengeEligible(t.Context(), "user", challengeCriteria{
		Kind: "single_guess_score", Score: 5000, Count: 1,
	}, time.Time{})
	if err != nil || !eligible {
		t.Fatalf("perfect guess eligibility = %t, %v", eligible, err)
	}
}

func TestScoreStreakTimeoutResetsProgress(t *testing.T) {
	repo, pool := mockPinsRepository(t)
	rows := pgxmock.NewRows([]string{"score", "timed_out"})
	for range 4 {
		rows.AddRow(5000, false)
	}
	rows.AddRow(0, true)
	for range 5 {
		rows.AddRow(4500, false)
	}
	pool.ExpectQuery("ORDER BY created_at, challenge_id, source_order").WithArgs("user").WillReturnRows(rows)
	eligible, err := repo.challengeEligible(t.Context(), "user", challengeCriteria{
		Kind: "score_streak", MinimumScore: 4000, Count: 5,
	}, time.Time{})
	if err != nil || !eligible {
		t.Fatalf("score streak after timeout = %t, %v", eligible, err)
	}
}

func TestScoreCountChallengeUsesMinimumScore(t *testing.T) {
	repo, pool := mockPinsRepository(t)
	pool.ExpectQuery("SELECT COUNT").WithArgs("user", 4500, 25).WillReturnRows(
		pgxmock.NewRows([]string{"eligible"}).AddRow(true),
	)
	eligible, err := repo.challengeEligible(t.Context(), "user", challengeCriteria{
		Kind: "score_count", MinimumScore: 4500, Count: 25,
	}, time.Time{})
	if err != nil || !eligible {
		t.Fatalf("high-score eligibility = %t, %v", eligible, err)
	}
}

func TestDistanceScoreChallengeUsesBothBounds(t *testing.T) {
	repo, pool := mockPinsRepository(t)
	pool.ExpectQuery("SELECT COUNT").WithArgs("user", 5000.0, 3000, 5).WillReturnRows(
		pgxmock.NewRows([]string{"eligible"}).AddRow(true),
	)
	eligible, err := repo.challengeEligible(t.Context(), "user", challengeCriteria{
		Kind: "distance_score_count", MinimumDistanceMeters: 5000.0,
		MinimumScore: 3000, Count: 5,
	}, time.Time{})
	if err != nil || !eligible {
		t.Fatalf("long-shot eligibility = %t, %v", eligible, err)
	}
}

func TestCloseCallChallengeUsesMaximumDistance(t *testing.T) {
	repo, pool := mockPinsRepository(t)
	pool.ExpectQuery("SELECT COUNT").WithArgs("user", 1000.0, 10).WillReturnRows(
		pgxmock.NewRows([]string{"eligible"}).AddRow(true),
	)
	eligible, err := repo.challengeEligible(t.Context(), "user", challengeCriteria{
		Kind: "distance_count", MaximumDistanceMeters: 1000.0, Count: 10,
	}, time.Time{})
	if err != nil || !eligible {
		t.Fatalf("close-call eligibility = %t, %v", eligible, err)
	}
}

func TestQuickDrawRequiresTimedPublicGuessesWithinWindow(t *testing.T) {
	repo, pool := mockPinsRepository(t)
	pool.ExpectQuery("g.created_at >= v.view_expires_at").WithArgs("user", 30, 10).WillReturnRows(
		pgxmock.NewRows([]string{"eligible"}).AddRow(true),
	)
	eligible, err := repo.challengeEligible(t.Context(), "user", challengeCriteria{
		Kind: "timed_public_speed", Seconds: 30, Count: 10,
	}, time.Time{})
	if err != nil || !eligible {
		t.Fatalf("quick-draw eligibility = %t, %v", eligible, err)
	}
}

func TestChallengeCriteriaRunTheirAggregateQueries(t *testing.T) {
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		criteria challengeCriteria
		args     []any
		columns  []string
		values   []any
		want     bool
	}{
		{"group count", challengeCriteria{Kind: "unique_groups", Count: 5}, []any{"user", 5}, []string{"eligible"}, []any{true}, true},
		{"public creators", challengeCriteria{Kind: "public_author_count", Count: 10}, []any{"user", 10}, []string{"eligible"}, []any{true}, true},
		{"received guesses", challengeCriteria{Kind: "received_public_guesses", Count: 100}, []any{"user", 100}, []string{"eligible"}, []any{true}, true},
		{"distinct guessers", challengeCriteria{Kind: "received_public_guessers", Count: 20}, []any{"user", 20}, []string{"eligible"}, []any{true}, true},
		{"received reactions", challengeCriteria{Kind: "received_public_reactions", Count: 100}, []any{"user", 100}, []string{"eligible"}, []any{true}, true},
		{"comments written", challengeCriteria{Kind: "authored_public_comments", Count: 50}, []any{"user", 50}, []string{"eligible"}, []any{true}, true},
		{"public guesses", challengeCriteria{Kind: "public_guess_count", Count: 100}, []any{"user", 100}, []string{"eligible"}, []any{true}, true},
		{"score count", challengeCriteria{Kind: "score_count", MinimumScore: 4500, Count: 25}, []any{"user", 4500, 25}, []string{"eligible"}, []any{true}, true},
		{"night points", challengeCriteria{Kind: "utc_hour_points", StartUTCHour: 0, EndUTCHour: 5, ThresholdExclusive: 9999}, []any{"user", 0, 5, 9999}, []string{"eligible"}, []any{true}, true},
		{"weekend points", challengeCriteria{Kind: "weekend_points", WeekStart: "monday_utc", ThresholdExclusive: 20000}, []any{"user", 20000}, []string{"eligible"}, []any{true}, true},
		{"lifetime points", challengeCriteria{Kind: "total_points", ThresholdExclusive: 100000}, []any{"user", 100000}, []string{"eligible"}, []any{true}, true},
		{"group and public scores", challengeCriteria{Kind: "source_score_count", MinimumScore: 4000, GroupCount: 10, PublicCount: 10}, []any{"user", 4000}, []string{"group_count", "public_count"}, []any{10, 10}, true},
		{"reaction support", challengeCriteria{Kind: "public_reaction_count", Count: 50}, []any{"user", 50}, []string{"eligible"}, []any{true}, true},
		{"commented posts", challengeCriteria{Kind: "public_commented_challenges", Count: 25}, []any{"user", 25}, []string{"eligible"}, []any{true}, true},
		{"weekly points", challengeCriteria{Kind: "weekly_points", WeekStart: "monday_utc", ThresholdExclusive: 20000}, []any{"user", 20000, now}, []string{"eligible"}, []any{true}, true},
		{"weekly winner", challengeCriteria{Kind: "group_weekly_rank", Rank: 1, TiePolicy: "shared_first", WeekStart: "monday_utc"}, []any{"user", now}, []string{"eligible"}, []any{true}, true},
		{"published locations", challengeCriteria{Kind: "published_public_locations", Count: 20}, []any{"user", 20}, []string{"eligible"}, []any{true}, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, pool := mockPinsRepository(t)
			pool.ExpectQuery("SELECT").WithArgs(test.args...).WillReturnRows(
				pgxmock.NewRows(test.columns).AddRow(test.values...),
			)
			eligible, err := repo.challengeEligible(t.Context(), "user", test.criteria, now)
			if err != nil || eligible != test.want {
				t.Fatalf("challenge eligibility = %t, %v; want %t", eligible, err, test.want)
			}
		})
	}
}
