package feed

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v5"
)

func TestAcceptTimedChallengeIsIdempotentAndReturnsMediaType(t *testing.T) {
	r, mock := mockRepository(t)
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	viewEnd := now.Add(10 * time.Second)
	guessEnd := viewEnd.Add(2 * time.Minute)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT p.user_id,p.mime_type").WithArgs("viewer", "post").WillReturnRows(
		pgxmock.NewRows([]string{"user_id", "mime_type"}).AddRow("author", "image/png"),
	)
	mock.ExpectExec("INSERT INTO public_challenge_views").WithArgs("post", "viewer", now, int64(10), int64(120)).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectQuery("SELECT accepted_at,media_delivered_at,view_expires_at,guess_expires_at").WithArgs("post", "viewer").WillReturnRows(
		pgxmock.NewRows([]string{"accepted_at", "media_delivered_at", "view_expires_at", "guess_expires_at"}).AddRow(now, nil, viewEnd, guessEnd),
	)
	mock.ExpectCommit()
	view, err := r.AcceptTimedChallenge(t.Context(), "post", "viewer", 10*time.Second, 2*time.Minute, now)
	if err != nil {
		t.Fatal(err)
	}
	if view.MediaType != "image/png" || view.Delivered || !view.ViewExpiresAt.Equal(viewEnd) || !view.GuessExpiresAt.Equal(guessEnd) {
		t.Fatalf("unexpected timed view: %+v", view)
	}
}

func TestTimedGuessPersistsTimeoutBeforeReturningDeadlineError(t *testing.T) {
	r, mock := mockRepository(t)
	now := time.Date(2026, 9, 21, 12, 5, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT p.user_id,p.lat,p.long").WithArgs("viewer", "post").WillReturnRows(
		pgxmock.NewRows([]string{"user_id", "lat", "long"}).AddRow("author", 48.8, 2.3),
	)
	mock.ExpectQuery("SELECT id,challenge_id,user_id,lat,long,score,distance,timed_out,created_at").WithArgs("post", "viewer").WillReturnError(pgx.ErrNoRows)
	mock.ExpectQuery("SELECT accepted_at,media_delivered_at,view_expires_at,guess_expires_at").WithArgs("post", "viewer").WillReturnRows(
		pgxmock.NewRows([]string{"accepted_at", "media_delivered_at", "view_expires_at", "guess_expires_at"}).AddRow(now.Add(-time.Minute), now.Add(-time.Minute), now.Add(-30*time.Second), now.Add(-time.Second)),
	)
	mock.ExpectExec("INSERT INTO public_guesses\\(id,challenge_id,user_id,lat,long,score,distance,timed_out,created_at\\)").WithArgs(pgxmock.AnyArg(), "post", "viewer", now).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	_, _, err := r.TimedGuess(t.Context(), "post", "viewer", 48.8, 2.3, now)
	if !errors.Is(err, ErrGuessTimeExpired) {
		t.Fatalf("TimedGuess error = %v, want timeout", err)
	}
}

func TestTimedGuessRejectsOwnerBeforeCreatingSessionGuess(t *testing.T) {
	r, mock := mockRepository(t)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT p.user_id,p.lat,p.long").WithArgs("viewer", "post").WillReturnRows(
		pgxmock.NewRows([]string{"user_id", "lat", "long"}).AddRow("viewer", 48.8, 2.3),
	)
	mock.ExpectRollback()
	if _, _, err := r.TimedGuess(t.Context(), "post", "viewer", 0, 0, time.Now()); !errors.Is(err, ErrOwnChallenge) {
		t.Fatalf("owner guess error = %v", err)
	}
}

func TestTimedResultsRequireResolutionForOtherViewers(t *testing.T) {
	r, mock := mockRepository(t)
	mock.ExpectQuery("SELECT p.user_id,p.lat,p.long").WithArgs("viewer", "post").WillReturnRows(
		pgxmock.NewRows([]string{"user_id", "lat", "long"}).AddRow("author", 48.8, 2.3),
	)
	mock.ExpectQuery("SELECT EXISTS \\(SELECT 1 FROM public_guesses").WithArgs("post", "viewer", pgxmock.AnyArg()).WillReturnRows(
		pgxmock.NewRows([]string{"exists"}).AddRow(false),
	)
	if _, err := r.TimedResults(t.Context(), "post", "viewer", time.Now()); !errors.Is(err, ErrForbidden) {
		t.Fatalf("unresolved results error = %v", err)
	}
}
