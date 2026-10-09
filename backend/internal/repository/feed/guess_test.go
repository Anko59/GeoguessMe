package feed

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v5"
)

func mockRepository(t *testing.T) (*Repository, pgxmock.PgxPoolIface) {
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
	return NewRepository(mock), mock
}

func TestGuessIsImmutableAndRejectsOwnChallenge(t *testing.T) {
	for _, tc := range []struct {
		name, owner string
		existing    bool
		wantErr     error
	}{
		{"new guess", "author", false, nil},
		{"repeat preserves original", "author", true, nil},
		{"own challenge", "viewer", false, ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, mock := mockRepository(t)
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT p.user_id,p.lat,p.long.*FOR KEY SHARE").WithArgs("viewer", "post").WillReturnRows(pgxmock.NewRows([]string{"owner", "lat", "long", "hide_location", "created_at"}).AddRow(tc.owner, 48.0, 2.0, false, time.Now()))
			if tc.wantErr != nil {
				mock.ExpectRollback()
			} else {
				inserted := int64(1)
				if tc.existing {
					inserted = 0
				}
				mock.ExpectExec("INSERT INTO public_guesses.*DO NOTHING").WithArgs("post", "viewer", 48.0, 2.0, 5000, 0.0).WillReturnResult(pgxmock.NewResult("INSERT", inserted))
				query := mock.ExpectQuery("SELECT score,distance,lat,long").WithArgs("post", "viewer")
				if tc.existing {
					query.WillReturnRows(pgxmock.NewRows([]string{"score", "distance", "lat", "long"}).AddRow(10, 120000.0, 47.0, 1.0))
				} else {
					query.WillReturnRows(pgxmock.NewRows([]string{"score", "distance", "lat", "long"}).AddRow(5000, 0.0, 48.0, 2.0))
				}
				mock.ExpectCommit()
			}
			result, err := r.Guess(t.Context(), "post", "viewer", 48, 2, time.Now(), 48*time.Hour)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error %v", err)
			}
			if err == nil && tc.existing && (result.Score != 10 || result.Lat != 47 || result.Long != 1) {
				t.Fatalf("duplicate overwritten: %+v", result)
			}
			if err == nil && !tc.existing && result.Score != 5000 {
				t.Fatalf("bad score: %+v", result)
			}
		})
	}
}

func TestGuessFailureCannotResolveChallenge(t *testing.T) {
	r, mock := mockRepository(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT p.user_id,p.lat,p.long").WithArgs("viewer", "post").WillReturnRows(pgxmock.NewRows([]string{"owner", "lat", "long", "hide_location", "created_at"}).AddRow("author", 0.0, 0.0, false, time.Now()))
	mock.ExpectExec("INSERT INTO public_guesses").WithArgs("post", "viewer", 0.0, 0.0, 5000, 0.0).WillReturnError(errors.New("insert failure"))
	mock.ExpectRollback()
	if _, err := r.Guess(t.Context(), "post", "viewer", 0, 0, time.Now(), 48*time.Hour); err == nil {
		t.Fatal("ignored insert failure")
	}
	for _, lat := range []float64{91, math.NaN(), math.Inf(1)} {
		if _, err := r.Guess(t.Context(), "post", "viewer", lat, 0, time.Now(), 48*time.Hour); err == nil {
			t.Fatalf("accepted %f", lat)
		}
	}
}

func TestResultRequiresViewerGuess(t *testing.T) {
	r, mock := mockRepository(t)
	mock.ExpectQuery("SELECT g.score,g.distance,g.lat,g.long,p.lat,p.long").WithArgs("unsolved", "post").WillReturnError(pgx.ErrNoRows)
	if _, err := r.Result(t.Context(), "post", "unsolved", time.Now(), 48*time.Hour); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unresolved result: %v", err)
	}
}

func TestResultAppliesConfiguredLocationPrivacy(t *testing.T) {
	created := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, viewer string
		hide         bool
		elapsed      time.Duration
		hidden       bool
	}{
		{"before deadline", "viewer", true, 29 * time.Minute, true},
		{"at deadline", "viewer", true, 30 * time.Minute, false},
		{"after deadline", "viewer", true, 31 * time.Minute, false},
		{"public location", "viewer", false, time.Minute, false},
		{"author exception", "author", true, time.Minute, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, mock := mockRepository(t)
			mock.ExpectQuery("SELECT g.score,g.distance,g.lat,g.long,p.lat,p.long").WithArgs(tc.viewer, "post").WillReturnRows(
				pgxmock.NewRows([]string{"score", "distance", "lat", "long", "actual_lat", "actual_long", "owner", "hide_location", "created_at"}).AddRow(4500, 100.0, 1.0, 2.0, 0.0, 0.0, "author", tc.hide, created),
			)
			result, err := r.Result(t.Context(), "post", tc.viewer, created.Add(tc.elapsed), 30*time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if result.LocationHidden != tc.hidden || result.Score != 4500 || result.Lat != 1 || result.Distance != 100 {
				t.Fatalf("result = %+v", result)
			}
			if tc.hidden {
				if result.ActualLat != nil || result.ActualLong != nil || result.LocationRevealsAt == nil || !result.LocationRevealsAt.Equal(created.Add(30*time.Minute)) {
					t.Fatalf("hidden answer = %+v", result)
				}
			} else if result.ActualLat == nil || result.ActualLong == nil || *result.ActualLat != 0 || *result.ActualLong != 0 || result.LocationRevealsAt != nil {
				t.Fatalf("revealed zero coordinates = %+v", result)
			}
		})
	}
}
