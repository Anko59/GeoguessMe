package atlas

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/google/uuid"
	"github.com/pashagolub/pgxmock/v4"
)

func TestLocationVisibility(t *testing.T) {
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name                                                     string
		owner, guessed, expired, hidden, revealBoundary, visible bool
	}{
		{name: "unplayed challenge"},
		{name: "author", owner: true, hidden: true, visible: true},
		{name: "guessed", guessed: true, visible: true},
		{name: "expiry boundary", expired: true, visible: true},
		{name: "hidden guessed", guessed: true, hidden: true},
		{name: "hidden expired", expired: true, hidden: true},
		{name: "hide boundary", guessed: true, hidden: true, revealBoundary: true, visible: true},
		{name: "hide ended but still unplayed", hidden: true, revealBoundary: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			photo := &models.Photo{ID: "photo", GroupID: "group", UserID: "poster", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), HideLocation: tc.hidden, Lat: 0, Long: 0}
			if tc.owner {
				photo.UserID = "viewer"
			}
			if tc.expired {
				photo.ExpiresAt = now
			}
			if tc.revealBoundary {
				photo.CreatedAt = now.Add(-48 * time.Hour)
			}
			got := visibleChallenge(photo, "Alice", "viewer", tc.guessed, now, 48*time.Hour)
			if (got.Lat != nil && got.Long != nil) != tc.visible {
				t.Fatalf("visibility: %+v", got)
			}
			body, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.visible && (strings.Contains(string(body), `"lat"`) || strings.Contains(string(body), `"long"`)) {
				t.Fatalf("coordinates leaked: %s", body)
			}
			if tc.visible && (*got.Lat != 0 || *got.Long != 0) {
				t.Fatal("zero coordinates were lost")
			}
		})
	}
}

func TestPaginationAndGroupIsolation(t *testing.T) {
	pool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	groupID := uuid.NewString()
	now := time.Date(2026, 9, 12, 12, 0, 0, 123000, time.UTC)
	rows := pgxmock.NewRows([]string{"id", "group_id", "user_id", "username", "created_at", "expires_at", "lat", "long", "hide_location", "guessed"})
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = uuid.NewString()
		rows.AddRow(ids[i], groupID, "poster", "Alice", now, now.Add(time.Hour), 48.0, 2.0, false, true)
	}
	pool.ExpectQuery(`WHERE p.group_id = \$1 AND EXISTS .*m.user_id = \$2.*ORDER BY p.created_at DESC, p.id DESC LIMIT 101`).WithArgs(groupID, "viewer").WillReturnRows(rows)
	page, err := List(context.Background(), pool, groupID, "viewer", "", now, 48*time.Hour)
	if err != nil || len(page.Items) != 100 || page.NextCursor == "" {
		t.Fatalf("page: %+v, %v", page, err)
	}
	at, id, err := decodeCursor(page.NextCursor, groupID)
	if err != nil || !at.Equal(now) || id != ids[99] {
		t.Fatalf("cursor: %s %s %v", at, id, err)
	}
	if _, _, err := decodeCursor(page.NextCursor, uuid.NewString()); !errors.Is(err, ErrInvalidCursor) {
		t.Fatal("cross-group cursor accepted")
	}
	pool.ExpectQuery(`AND \(p.created_at, p.id\) < \(\$3, \$4\).*ORDER BY p.created_at DESC, p.id DESC LIMIT 101`).WithArgs(groupID, "viewer", now, ids[99]).WillReturnRows(pgxmock.NewRows([]string{"id", "group_id", "user_id", "username", "created_at", "expires_at", "lat", "long", "hide_location", "guessed"}).AddRow(ids[100], groupID, "poster", "Alice", now, now, 48.0, 2.0, false, false))
	last, err := List(context.Background(), pool, groupID, "viewer", page.NextCursor, now, 48*time.Hour)
	if err != nil || len(last.Items) != 1 || last.NextCursor != "" {
		t.Fatalf("last: %+v, %v", last, err)
	}
	if err := pool.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCursorAndDatabaseFailures(t *testing.T) {
	groupID := uuid.NewString()
	for _, cursor := range []string{"!", strings.Repeat("a", 513), base64.RawURLEncoding.EncodeToString([]byte(groupID + "|invalid|" + uuid.NewString())), base64.RawURLEncoding.EncodeToString([]byte(groupID + "|2026-09-12T12:00:00Z|invalid"))} {
		if _, err := List(context.Background(), nil, groupID, "viewer", cursor, time.Now(), time.Hour); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("cursor accepted: %s", cursor)
		}
	}
	for _, failure := range []string{"query", "scan", "rows"} {
		t.Run(failure, func(t *testing.T) {
			pool, err := pgxmock.NewPool()
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			expectation := pool.ExpectQuery("SELECT p.id").WithArgs(groupID, "viewer")
			if failure == "query" {
				expectation.WillReturnError(errors.New("unavailable"))
			} else {
				rows := pgxmock.NewRows([]string{"wrong column"}).AddRow("invalid")
				if failure == "rows" {
					rows.RowError(0, errors.New("interrupted"))
				}
				expectation.WillReturnRows(rows)
			}
			if _, err := List(context.Background(), pool, groupID, "viewer", "", time.Now(), time.Hour); err == nil {
				t.Fatal("database error was ignored")
			}
			if err := pool.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
