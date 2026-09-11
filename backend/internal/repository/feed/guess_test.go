package feed

import (
	"errors"
	"math"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
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
			mock.ExpectQuery("SELECT user_id,lat,long.*FOR KEY SHARE").WithArgs("post").WillReturnRows(pgxmock.NewRows([]string{"owner", "lat", "long"}).AddRow(tc.owner, 48.0, 2.0))
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
			result, err := r.Guess(t.Context(), "post", "viewer", 48, 2)
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
	mock.ExpectQuery("SELECT user_id,lat,long").WithArgs("post").WillReturnRows(pgxmock.NewRows([]string{"owner", "lat", "long"}).AddRow("author", 0.0, 0.0))
	mock.ExpectExec("INSERT INTO public_guesses").WithArgs("post", "viewer", 0.0, 0.0, 5000, 0.0).WillReturnError(errors.New("insert failure"))
	mock.ExpectRollback()
	if _, err := r.Guess(t.Context(), "post", "viewer", 0, 0); err == nil {
		t.Fatal("ignored insert failure")
	}
	for _, lat := range []float64{91, math.NaN(), math.Inf(1)} {
		if _, err := r.Guess(t.Context(), "post", "viewer", lat, 0); err == nil {
			t.Fatalf("accepted %f", lat)
		}
	}
}

func TestResultRequiresViewerGuess(t *testing.T) {
	r, mock := mockRepository(t)
	mock.ExpectQuery("SELECT g.score,g.distance,g.lat,g.long,p.lat,p.long").WithArgs("post", "unsolved").WillReturnError(pgx.ErrNoRows)
	if _, err := r.Result(t.Context(), "post", "unsolved"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unresolved result: %v", err)
	}
}
