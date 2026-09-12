package feed

import (
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
)

func TestFeedCursorKeepsTimestampTiesAndExcludesLookahead(t *testing.T) {
	r, mock := mockRepository(t)
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	first := "00000000-0000-0000-0000-000000000003"
	second := "00000000-0000-0000-0000-000000000002"
	third := "00000000-0000-0000-0000-000000000001"
	rows := pgxmock.NewRows([]string{"id", "user", "username", "caption", "at", "owner", "resolved", "likes", "liked", "comments"})
	for _, id := range []string{first, second, third} {
		rows.AddRow(id, "author", "Explorer", "", now, false, false, 0, false, 0)
	}
	mock.ExpectQuery("ORDER BY p.created_at DESC,p.id DESC LIMIT").WithArgs("viewer", 3).WillReturnRows(rows)
	page, err := r.List(t.Context(), "viewer", Cursor{}, 2)
	if err != nil || len(page.Items) != 2 {
		t.Fatalf("page %+v, %v", page, err)
	}
	cursor, err := ParseCursor(page.NextCursor)
	if err != nil || cursor.ID != second || !cursor.CreatedAt.Equal(now) {
		t.Fatalf("cursor %+v, %v", cursor, err)
	}
	mock.ExpectQuery("WHERE .*ORDER BY p.created_at DESC,p.id DESC LIMIT").WithArgs("viewer", 3, now, second).WillReturnRows(pgxmock.NewRows([]string{"id", "user", "username", "caption", "at", "owner", "resolved", "likes", "liked", "comments"}).AddRow(third, "author", "Explorer", "", now, false, false, 0, false, 0))
	page, err = r.List(t.Context(), "viewer", cursor, 2)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != third || page.NextCursor != "" {
		t.Fatalf("last page %+v, %v", page, err)
	}
}
