package feed

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pashagolub/pgxmock/v5"
)

func TestCreateFriendsPostValidatesAndStoresSelectedGroupsAtomically(t *testing.T) {
	r, mock := mockRepository(t)
	created := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	groupIDs := []string{"00000000-0000-0000-0000-000000000002"}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO public_challenges").WithArgs("post", "author", "A place", "friends", "key", "image/png", []byte("preview"), 48.8, 2.3, false, int64(0), "", "", created).WillReturnResult(pgxmock.NewResult("INSERT", 1))
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
	mock.ExpectExec("INSERT INTO public_challenges").WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
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
	rows := pgxmock.NewRows([]string{"id", "user", "username", "avatar", "caption", "at", "audience", "owner", "resolved", "likes", "liked", "comments"})
	for _, id := range []string{first, second, third} {
		rows.AddRow(id, "author", "Explorer", "avatar.png", "", now, "public", false, false, 0, false, 0)
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
	mock.ExpectQuery("WHERE .*ORDER BY p.created_at DESC,p.id DESC LIMIT").WithArgs("viewer", 3, now, second).WillReturnRows(pgxmock.NewRows([]string{"id", "user", "username", "avatar", "caption", "at", "audience", "owner", "resolved", "likes", "liked", "comments"}).AddRow(third, "author", "Explorer", "avatar.png", "", now, "public", false, false, 0, false, 0))
	page, err = r.List(t.Context(), "viewer", cursor, 2)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != third || page.NextCursor != "" {
		t.Fatalf("last page %+v, %v", page, err)
	}
}

func TestChallengeVisibilitySymmetricAndOutsideAudienceAlternatives(t *testing.T) {
	for _, predicate := range []string{"b.blocker_id=$1 AND b.blocked_id=p.user_id", "b.blocker_id=p.user_id AND b.blocked_id=$1"} {
		if !strings.Contains(challengeVisibility, predicate) {
			t.Fatalf("missing block direction: %s", challengeVisibility)
		}
	}
	if !strings.HasPrefix(challengeVisibility, "NOT EXISTS") || !strings.Contains(challengeVisibility, ")) AND (p.user_id=$1") {
		t.Fatalf("audience branches must not bypass blocking: %s", challengeVisibility)
	}
}

func TestBlockedPublicChallengeUnavailableAcrossContentPaths(t *testing.T) {
	now := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	for _, operation := range []string{"get", "media", "guess", "comment", "comments", "react", "accept", "timed-guess", "timeout", "results"} {
		t.Run(operation, func(t *testing.T) {
			r, mock := mockRepository(t)
			transaction := operation == "guess" || operation == "accept" || operation == "timed-guess" || operation == "timeout"
			if transaction {
				mock.ExpectBegin()
			}
			query := mock.ExpectQuery(regexp.QuoteMeta(challengeVisibility))
			switch operation {
			case "comments", "react":
				query.WithArgs("viewer", "blocked").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
			case "media":
				query.WithArgs("viewer", "blocked", now).WillReturnError(pgx.ErrNoRows)
			case "comment":
				query.WithArgs("viewer", pgxmock.AnyArg(), "hello", "blocked").WillReturnError(pgx.ErrNoRows)
			default:
				query.WithArgs("viewer", "blocked").WillReturnError(pgx.ErrNoRows)
			}
			if transaction {
				mock.ExpectRollback()
			}
			var err error
			switch operation {
			case "get":
				_, err = r.Get(t.Context(), "blocked", "viewer")
			case "media":
				_, err = r.Media(t.Context(), "blocked", "viewer", now)
			case "guess":
				_, err = r.Guess(t.Context(), "blocked", "viewer", 0, 0, now, 48*time.Hour)
			case "comment":
				_, err = r.Comment(t.Context(), "blocked", "viewer", "hello")
			case "comments":
				_, err = r.Comments(t.Context(), "blocked", "viewer", Cursor{}, 2)
			case "react":
				err = r.React(t.Context(), "blocked", "viewer", true)
			case "accept":
				_, err = r.AcceptTimedChallenge(t.Context(), "blocked", "viewer", time.Second, time.Minute, now)
			case "timed-guess":
				_, _, err = r.TimedGuess(t.Context(), "blocked", "viewer", 0, 0, now)
			case "timeout":
				_, _, err = r.TimedTimeout(t.Context(), "blocked", "viewer", now)
			case "results":
				_, err = r.TimedResults(t.Context(), "blocked", "viewer", now, 48*time.Hour)
			}
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("blocked %s error = %v", operation, err)
			}
		})
	}
}

func TestFeedListFiltersBlockedAuthorsBeforePagination(t *testing.T) {
	r, mock := mockRepository(t)
	mock.ExpectQuery(regexp.QuoteMeta(challengeVisibility)+`.*ORDER BY.*LIMIT`).WithArgs("viewer", 3).WillReturnRows(pgxmock.NewRows([]string{"id", "user", "username", "avatar", "caption", "at", "audience", "owner", "resolved", "likes", "liked", "comments"}))
	page, err := r.List(t.Context(), "viewer", Cursor{}, 2)
	if err != nil || len(page.Items) != 0 || page.NextCursor != "" {
		t.Fatalf("blocked author list = %+v, %v", page, err)
	}
}

func TestPostAggregatesExcludeBlockedContributors(t *testing.T) {
	if !strings.Contains(selectPost, "WHERE r.challenge_id=p.id AND "+reactionVisibility) || !strings.Contains(selectPost, "WHERE c.challenge_id=p.id AND "+commentVisibility) {
		t.Fatalf("post counts include hidden contributors: %s", selectPost)
	}
}
