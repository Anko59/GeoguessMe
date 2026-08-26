package party

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func newMockPool(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
	})
	return mock
}

func windowRows(w Window) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "group_id", "started_by", "started_at", "ends_at"}).
		AddRow(w.ID, w.GroupID, w.StartedBy, w.StartedAt, w.EndsAt)
}

func TestWindowActiveAndNextAvailable(t *testing.T) {
	started := time.Date(2025, 6, 20, 20, 0, 0, 0, time.UTC)
	window := Window{StartedAt: started, EndsAt: started.Add(time.Hour)}
	if !window.Active(started.Add(30 * time.Minute)) {
		t.Fatal("window must be active mid-way")
	}
	if window.Active(started.Add(-time.Second)) || window.Active(started.Add(time.Hour)) {
		t.Fatal("window must be inactive before start and at end")
	}
	if got := window.NextAvailableAt(48 * time.Hour); !got.Equal(started.Add(time.Hour + 48*time.Hour)) {
		t.Fatalf("next available = %v, want ends_at plus cooldown", got)
	}
	if got := window.NextAvailableAt(0); !got.Equal(window.EndsAt) {
		t.Fatalf("zero cooldown next available = %v, want ends_at", got)
	}
}

func TestStartCreatesWindow(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC().Truncate(time.Microsecond)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM groups").WithArgs("group-1").
		WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("group-1"))
	mock.ExpectQuery("SELECT id, group_id, started_by").WithArgs("group-1", now).WillReturnError(pgx.ErrNoRows)
	mock.ExpectExec("INSERT INTO group_party_times").
		WithArgs(pgxmock.AnyArg(), "group-1", "user-1", now, now.Add(time.Hour)).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	window, err := repo.Start(context.Background(), "group-1", "user-1", now, time.Hour, 48*time.Hour)
	if err != nil {
		t.Fatalf("start = %v", err)
	}
	if window.ID == "" || window.GroupID != "group-1" || window.StartedBy != "user-1" {
		t.Fatalf("window = %+v", window)
	}
	if !window.EndsAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("ends_at = %v, want start plus duration", window.EndsAt)
	}
}

func TestStartRules(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	duration, cooldown := time.Hour, 48*time.Hour
	cases := []struct {
		name    string
		latest  *Window
		wantErr error
	}{
		{
			name:    "active party blocks a second start",
			latest:  &Window{ID: "w1", GroupID: "group-1", StartedAt: now.Add(-30 * time.Minute), EndsAt: now.Add(30 * time.Minute)},
			wantErr: ErrPartyActive,
		},
		{
			name:    "recharging cooldown blocks an early start",
			latest:  &Window{ID: "w2", GroupID: "group-1", StartedAt: now.Add(-2 * time.Hour), EndsAt: now.Add(-time.Hour)},
			wantErr: ErrPartyRecharging,
		},
		{name: "elapsed cooldown allows a start", latest: &Window{ID: "w3", GroupID: "group-1", StartedAt: now.Add(-72 * time.Hour), EndsAt: now.Add(-71 * time.Hour)}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			mock := newMockPool(t)
			repo := NewRepository(mock)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT id FROM groups").WithArgs("group-1").
				WillReturnRows(pgxmock.NewRows([]string{"id"}).AddRow("group-1"))
			mock.ExpectQuery("SELECT id, group_id, started_by").WithArgs("group-1", now).WillReturnRows(windowRows(*testCase.latest))
			if testCase.wantErr == nil {
				mock.ExpectExec("INSERT INTO group_party_times").
					WithArgs(pgxmock.AnyArg(), "group-1", "user-1", now, now.Add(duration)).
					WillReturnResult(pgxmock.NewResult("INSERT", 1))
				mock.ExpectCommit()
			} else {
				mock.ExpectRollback()
			}
			_, err := repo.Start(context.Background(), "group-1", "user-1", now, duration, cooldown)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("start error = %v, want %v", err, testCase.wantErr)
			}
		})
	}
}

func TestStartUnknownGroup(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM groups").WithArgs("missing").WillReturnError(pgx.ErrNoRows)
	mock.ExpectRollback()
	if _, err := repo.Start(context.Background(), "missing", "user-1", time.Now(), time.Hour, 48*time.Hour); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown group error = %v, want ErrNotFound", err)
	}
}

func TestStatusReturnsLatestStartedWindow(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC()
	latest := Window{ID: "w9", GroupID: "group-1", StartedBy: "user-1", StartedAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour)}
	mock.ExpectQuery("SELECT id, group_id, started_by").WithArgs("group-1", now).WillReturnRows(windowRows(latest))
	got, err := repo.Status(context.Background(), "group-1", now)
	if err != nil || got == nil || got.ID != "w9" {
		t.Fatalf("status = %+v, %v", got, err)
	}

	mock.ExpectQuery("SELECT id, group_id, started_by").WithArgs("group-1", now).WillReturnError(pgx.ErrNoRows)
	got, err = repo.Status(context.Background(), "group-1", now)
	if err != nil || got != nil {
		t.Fatalf("empty status = %+v, %v", got, err)
	}
}

func TestScoreMultiplier(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name         string
		rows         *pgxmock.Rows
		queryErr     error
		wantFactor   int
		wantDoubled  bool
		wantFailHard bool
	}{
		{
			name:        "no active party scores normally",
			queryErr:    pgx.ErrNoRows,
			wantFactor:  1,
			wantDoubled: false,
		},
		{
			name:        "active party with a posted challenge doubles",
			rows:        pgxmock.NewRows([]string{"exists"}).AddRow(true),
			wantFactor:  Multiplier,
			wantDoubled: true,
		},
		{
			name:        "active party without a posted challenge scores normally",
			rows:        pgxmock.NewRows([]string{"exists"}).AddRow(false),
			wantFactor:  1,
			wantDoubled: false,
		},
		{
			name:         "lookup failure fails hard",
			queryErr:     errors.New("connection refused"),
			wantFactor:   0,
			wantFailHard: true,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			mock := newMockPool(t)
			expect := mock.ExpectQuery("EXISTS").WithArgs("group-1", "user-2", now)
			switch {
			case testCase.queryErr != nil:
				expect.WillReturnError(testCase.queryErr)
			default:
				expect.WillReturnRows(testCase.rows)
			}
			factor, doubled, err := ScoreMultiplier(context.Background(), mock, "group-1", "user-2", now)
			if testCase.wantFailHard && err == nil {
				t.Fatal("expected a hard failure")
			}
			if !testCase.wantFailHard && (err != nil || factor != testCase.wantFactor || doubled != testCase.wantDoubled) {
				t.Fatalf("multiplier = (%d, %v, %v), want (%d, %v, nil)", factor, doubled, err, testCase.wantFactor, testCase.wantDoubled)
			}
		})
	}
}
