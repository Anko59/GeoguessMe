package feed

import (
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v5"
)

func TestCreateFriendsPostValidatesAndStoresSelectedGroupsAtomically(t *testing.T) {
	r, mock := mockRepository(t)
	created := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	groupIDs := []string{"00000000-0000-0000-0000-000000000002"}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public_challenges").WithArgs("post", "author", "A place", "friends", "key", "image/png", []byte("preview"), 48.8, 2.3, created).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM group_members").WithArgs("author", groupIDs).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec("INSERT INTO public_challenge_groups").WithArgs("post", groupIDs).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	if err := r.Create(t.Context(), NewChallenge{ID: "post", UserID: "author", Caption: "A place", Audience: "friends", GroupIDs: groupIDs, StorageKey: "key", MIMEType: "image/png", Preview: []byte("preview"), Lat: 48.8, Long: 2.3, CreatedAt: created}); err != nil {
		t.Fatalf("create friends post = %v", err)
	}
}

func TestCreateFriendsPostRejectsUnownedSelectedGroup(t *testing.T) {
	r, mock := mockRepository(t)
	groupIDs := []string{"00000000-0000-0000-0000-000000000002"}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public_challenges").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM group_members").WithArgs("author", groupIDs).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectRollback()
	if err := r.Create(t.Context(), NewChallenge{ID: "post", UserID: "author", Audience: "friends", GroupIDs: groupIDs}); err == nil || !errors.Is(err, ErrForbidden) {
		t.Fatalf("unowned group error = %v, want ErrForbidden", err)
	}
}

func TestFeedCursorKeepsTimestampTiesAndExcludesLookahead(t *testing.T) {
	r, mock := mockRepository(t)
	now := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	first := "00000000-0000-0000-0000-000000000003"
	second := "00000000-0000-0000-0000-000000000002"
	third := "00000000-0000-0000-0000-000000000001"
	rows := pgxmock.NewRows([]string{"id", "user", "username", "caption", "at", "audience", "owner", "resolved", "likes", "liked", "comments"})
	for _, id := range []string{first, second, third} {
		rows.AddRow(id, "author", "Explorer", "", now, "public", false, false, 0, false, 0)
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
	mock.ExpectQuery("WHERE .*ORDER BY p.created_at DESC,p.id DESC LIMIT").WithArgs("viewer", 3, now, second).WillReturnRows(pgxmock.NewRows([]string{"id", "user", "username", "caption", "at", "audience", "owner", "resolved", "likes", "liked", "comments"}).AddRow(third, "author", "Explorer", "", now, "public", false, false, 0, false, 0))
	page, err = r.List(t.Context(), "viewer", cursor, 2)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != third || page.NextCursor != "" {
		t.Fatalf("last page %+v, %v", page, err)
	}
}
