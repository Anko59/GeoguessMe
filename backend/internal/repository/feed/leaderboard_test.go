package feed

import (
	"errors"
	"testing"

	"github.com/pashagolub/pgxmock/v5"
)

func TestLeaderboardRanksTiesAndUsesStableCursorPagination(t *testing.T) {
	r, mock := mockRepository(t)
	query := "WITH totals AS"
	mock.ExpectQuery(query).WithArgs(3).WillReturnRows(
		pgxmock.NewRows([]string{"user_id", "username", "avatar", "total_score", "rank"}).
			AddRow("user-a", "Alice", "avatar.png", 5000, 1).
			AddRow("user-b", "Bob", "avatar2.png", 5000, 1).
			AddRow("user-c", "Carol", "avatar3.png", 4000, 3),
	)

	page, err := r.Leaderboard(t.Context(), LeaderboardCursor{}, 2)
	if err != nil {
		t.Fatalf("Leaderboard first page = %v", err)
	}
	if len(page.Items) != 2 || page.Items[0].Username != "Alice" || page.Items[1].Username != "Bob" {
		t.Fatalf("first page = %+v", page)
	}
	if page.Items[0].Rank != 1 || page.Items[1].Rank != 1 || page.NextCursor == "" {
		t.Fatalf("first page ranking/cursor = %+v", page)
	}
	cursor, err := ParseLeaderboardCursor(page.NextCursor)
	if err != nil || cursor.Score != 5000 || cursor.Username != "Bob" || cursor.UserID != "user-b" {
		t.Fatalf("cursor = %+v, %v", cursor, err)
	}
	mock.ExpectQuery(query).WithArgs(5000, "Bob", "user-b", 3).WillReturnRows(
		pgxmock.NewRows([]string{"user_id", "username", "avatar", "total_score", "rank"}).AddRow("user-c", "Carol", "avatar3.png", 4000, 3),
	)

	second, err := r.Leaderboard(t.Context(), cursor, 2)
	if err != nil {
		t.Fatalf("Leaderboard second page = %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].Username != "Carol" || second.Items[0].Rank != 3 || second.NextCursor != "" {
		t.Fatalf("second page = %+v", second)
	}
}

func TestProfileLeaderboardScopesChallengesToVisibleProfileAndExcludesOwner(t *testing.T) {
	r, mock := mockRepository(t)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("owner").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("(?s)WITH totals AS.*p.user_id=\\$2.*g.user_id<>\\$2").WithArgs("owner", "owner", 3).WillReturnRows(
		pgxmock.NewRows([]string{"user_id", "username", "avatar", "total_score", "rank"}).
			AddRow("guesser-a", "Alice", "avatar.png", 5000, 1).
			AddRow("guesser-b", "Bob", "avatar2.png", 4000, 2).
			AddRow("owner", "Owner", "avatar3.png", 3000, 3),
	)
	page, err := r.ProfileLeaderboard(t.Context(), "owner", "owner", LeaderboardCursor{}, 2)
	if err != nil {
		t.Fatalf("ProfileLeaderboard = %v", err)
	}
	if len(page.Items) != 2 || page.Items[0].UserID != "guesser-a" || page.Items[1].UserID != "guesser-b" {
		t.Fatalf("page = %+v", page)
	}
	if page.Items[0].Avatar != "avatar.png" || page.NextCursor == "" {
		t.Fatalf("avatar/cursor = %+v", page)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileLeaderboardRejectsUnknownOrHiddenProfiles(t *testing.T) {
	for _, tc := range []struct {
		name       string
		exists     bool
		shared     bool
		wantErr    error
		wantShared bool
	}{
		{name: "unknown", exists: false, wantErr: ErrProfileNotFound},
		{name: "hidden", exists: true, shared: false, wantErr: ErrForbidden, wantShared: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, mock := mockRepository(t)
			mock.ExpectQuery("SELECT EXISTS").WithArgs("owner").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(tc.exists))
			if tc.wantShared {
				mock.ExpectQuery("SELECT EXISTS").WithArgs("owner", "viewer").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(tc.shared))
			}
			_, err := r.ProfileLeaderboard(t.Context(), "owner", "viewer", LeaderboardCursor{}, 20)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
