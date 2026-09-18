package feed

import (
	"testing"

	"github.com/pashagolub/pgxmock/v5"
)

func TestLeaderboardRanksTiesAndUsesStableCursorPagination(t *testing.T) {
	r, mock := mockRepository(t)
	query := "WITH totals AS"
	mock.ExpectQuery(query).WithArgs(3).WillReturnRows(
		pgxmock.NewRows([]string{"user_id", "username", "total_score", "rank"}).
			AddRow("user-a", "Alice", 5000, 1).
			AddRow("user-b", "Bob", 5000, 1).
			AddRow("user-c", "Carol", 4000, 3),
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
		pgxmock.NewRows([]string{"user_id", "username", "total_score", "rank"}).AddRow("user-c", "Carol", 4000, 3),
	)

	second, err := r.Leaderboard(t.Context(), cursor, 2)
	if err != nil {
		t.Fatalf("Leaderboard second page = %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].Username != "Carol" || second.Items[0].Rank != 3 || second.NextCursor != "" {
		t.Fatalf("second page = %+v", second)
	}
}
