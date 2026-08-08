package repository

import (
	"context"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
)

// TestProfileSeamAdapters pins the profile/auth seam adapters that the
// not-yet-migrated profile handler still calls (PR 7 removes them): each
// delegates to the groups persistence slice over the global pool. Covering
// them here keeps the parent package above its coverage floor while the
// gameplay slice lives in internal/repository/groups.
func TestProfileSeamAdapters(t *testing.T) {
	mock := newMockPool(t)
	ctx := context.Background()

	mock.ExpectQuery("SELECT EXISTS").WithArgs("g1", "u1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	member, err := IsGroupMemberContext(ctx, "g1", "u1")
	if err != nil || !member {
		t.Fatalf("IsGroupMemberContext = %v, %v", member, err)
	}

	mock.ExpectQuery("SELECT EXISTS").WithArgs("u1", "u2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	shared, err := SharesGroupContext(ctx, "u1", "u2")
	if err != nil || !shared {
		t.Fatalf("SharesGroupContext = %v, %v", shared, err)
	}

	mock.ExpectQuery(`(?s)WITH scores AS.*ranked`).WithArgs("u1").
		WillReturnRows(pgxmock.NewRows([]string{"rank", "total_players"}).AddRow(int64(1), int64(3)))
	stats, err := GetGlobalAverageRankContext(ctx, "u1")
	if err != nil || stats.Rank != 1 || stats.TotalPlayers != 3 {
		t.Fatalf("GetGlobalAverageRankContext = %+v, %v", stats, err)
	}

	challengeRows := func() *pgxmock.Rows {
		return pgxmock.NewRows([]string{"id", "created_at", "user_id", "score"}).
			AddRow("p1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "u1", 4000).
			AddRow("p1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "u2", 1000)
	}
	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at, g\.user_id, g\.score.*WHERE TRUE ORDER BY`).
		WillReturnRows(challengeRows())
	elo, err := GetGlobalEloContext(ctx, "u1")
	if err != nil || elo.Elo <= 0 || elo.Rank != 1 || elo.TotalPlayers != 2 {
		t.Fatalf("GetGlobalEloContext = %+v, %v", elo, err)
	}

	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at, g\.user_id, g\.score.*WHERE TRUE ORDER BY`).
		WillReturnRows(challengeRows())
	challenges, err := LoadGlobalChallengesContext(ctx)
	if err != nil || len(challenges) != 1 {
		t.Fatalf("LoadGlobalChallengesContext = %d, %v", len(challenges), err)
	}

	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at, g\.user_id, g\.score.*WHERE TRUE AND g\.group_id = \$1`).
		WithArgs("g1").
		WillReturnRows(challengeRows())
	groupChallenges, err := LoadChallengesContext(ctx, "g1", nil)
	if err != nil || len(groupChallenges) != 1 {
		t.Fatalf("LoadChallengesContext = %d, %v", len(groupChallenges), err)
	}

	// NewRepository + the UserGroups delegate over the injected pool.
	repos := NewRepository(mock)
	mock.ExpectQuery("SELECT g.id, g.name, g.code").WithArgs("u1").
		WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow("g1", "One", "AAA", time.Now()))
	groups, err := repos.UserGroups(ctx, "u1")
	if err != nil || len(groups) != 1 {
		t.Fatalf("UserGroups = %d, %v", len(groups), err)
	}
}
