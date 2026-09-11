package feed

import (
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
)

func TestReactionIsIdempotentAndReportsMissingPosts(t *testing.T) {
	r, mock := mockRepository(t)
	for range 2 {
		mock.ExpectQuery("WITH challenge AS.*DO NOTHING").WithArgs("post", "viewer").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		if err := r.React(t.Context(), "post", "viewer", true); err != nil {
			t.Fatal(err)
		}
	}
	mock.ExpectQuery("WITH challenge AS").WithArgs("missing", "viewer").
		WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	if err := r.React(t.Context(), "missing", "viewer", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing post: %v", err)
	}
	for range 2 {
		mock.ExpectExec("DELETE FROM public_reactions").WithArgs("post", "viewer").WillReturnResult(pgxmock.NewResult("DELETE", 0))
		if err := r.React(t.Context(), "post", "viewer", false); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCommentsPagePreservesTiesAndModeratorPermissions(t *testing.T) {
	r, mock := mockRepository(t)
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	first := "00000000-0000-0000-0000-000000000003"
	second := "00000000-0000-0000-0000-000000000002"
	columns := []string{"id", "user", "name", "content", "at", "can_delete"}
	mock.ExpectQuery("SELECT EXISTS").WithArgs("post").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT c.id,c.user_id").WithArgs("post", "viewer", 2).
		WillReturnRows(pgxmock.NewRows(columns).AddRow(first, "author", "Explorer", "One", now, true).AddRow(second, "author", "Explorer", "Two", now, false))
	page, err := r.Comments(t.Context(), "post", "viewer", Cursor{}, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != first || !page.Items[0].CanDelete {
		t.Fatalf("first page: %+v, %v", page, err)
	}
	cursor, err := ParseCursor(page.NextCursor)
	if err != nil || cursor.ID != first || !cursor.CreatedAt.Equal(now) {
		t.Fatalf("cursor: %+v, %v", cursor, err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs("post").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT c.id,c.user_id.*AND .*ORDER BY").WithArgs("post", "viewer", 2, now, first).
		WillReturnRows(pgxmock.NewRows(columns).AddRow(second, "author", "Explorer", "Two", now, false))
	page, err = r.Comments(t.Context(), "post", "viewer", cursor, 1)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != second || page.Items[0].CanDelete || page.NextCursor != "" {
		t.Fatalf("last page: %+v, %v", page, err)
	}
}

func TestCommentsPropagateMissingPostsAndReadFailures(t *testing.T) {
	r, mock := mockRepository(t)
	mock.ExpectQuery("SELECT EXISTS").WithArgs("missing").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	if _, err := r.Comments(t.Context(), "missing", "viewer", Cursor{}, 20); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing post: %v", err)
	}
	failure := errors.New("database disconnected")
	mock.ExpectQuery("SELECT EXISTS").WithArgs("post").WillReturnError(failure)
	if _, err := r.Comments(t.Context(), "post", "viewer", Cursor{}, 20); !errors.Is(err, failure) {
		t.Fatalf("lost failure: %v", err)
	}
}
