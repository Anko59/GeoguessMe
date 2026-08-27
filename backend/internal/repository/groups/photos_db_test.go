package groups

import (
	"context"
	"errors"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func photoRows(photo *models.Photo) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "user_id", "group_id", "url", "storage_key", "mime_type", "byte_size", "lat", "long", "lifecycle_status", "hide_location", "created_at", "expires_at", "retention_at"}).
		AddRow(photo.ID, photo.UserID, photo.GroupID, photo.URL, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.Lat, photo.Long, photo.LifecycleStatus, photo.HideLocation, photo.CreatedAt, photo.ExpiresAt, photo.RetentionAt)
}

func guessRows(now time.Time) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "photo_id", "user_id", "group_id", "lat", "long", "score", "distance", "timed_out", "created_at"}).
		AddRow("guess-1", "photo-1", "user-2", "group-1", 48.8, 2.3, 90, 10.5, false, now)
}

func TestPhotoCreationAndChallengeAcceptance(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC().Truncate(time.Microsecond)
	photo := &models.Photo{ID: "photo-1", UserID: "user-1", GroupID: "group-1", StorageKey: "photos/one", MIMEType: "image/jpeg", ByteSize: 10, Lat: 48.8, Long: 2.3, LifecycleStatus: "ready", CreatedAt: now, ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO photos").WithArgs(photo.ID, photo.UserID, photo.GroupID, photo.URL, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.Lat, photo.Long, photo.LifecycleStatus, photo.HideLocation, photo.CreatedAt, photo.ExpiresAt, photo.RetentionAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	if err := repo.CreatePhotos(context.Background(), []*models.Photo{photo}); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(photoRows(photo))
	got, err := repo.Photo(context.Background(), photo.ID)
	if err != nil || got == nil || got.StorageKey != photo.StorageKey {
		t.Fatalf("photo = %+v, %v", got, err)
	}
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs("missing").WillReturnError(pgx.ErrNoRows)
	got, err = repo.Photo(context.Background(), "missing")
	if err != nil || got != nil {
		t.Fatalf("missing photo = %+v, %v", got, err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, user_id, group_id.*FOR UPDATE").WithArgs(photo.ID).WillReturnRows(photoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT photo_id, user_id").WithArgs(photo.ID, "user-2").WillReturnError(pgx.ErrNoRows)
	viewExpires := now.Add(30 * time.Minute)
	guessExpires := viewExpires.Add(2 * time.Minute)
	mock.ExpectExec("INSERT INTO challenge_views").WithArgs(photo.ID, "user-2", now, viewExpires, guessExpires).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	acceptedPhoto, view, err := repo.AcceptChallenge(context.Background(), photo.ID, "user-2", 30*time.Minute, 2*time.Minute, now)
	if err != nil || acceptedPhoto.ID != photo.ID || view.ViewExpiresAt != viewExpires || view.GuessExpiresAt != guessExpires {
		t.Fatalf("accepted = %+v/%+v, %v", acceptedPhoto, view, err)
	}

	// Re-accepting an existing legacy view row (pre-migration, NULL guess
	// deadline) derives the guess deadline from the stored view end so the
	// accept response stays valid.
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, user_id, group_id.*FOR UPDATE").WithArgs(photo.ID).WillReturnRows(photoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT photo_id, user_id").WithArgs(photo.ID, "user-2").
		WillReturnRows(pgxmock.NewRows([]string{"photo_id", "user_id", "accepted_at", "view_expires_at", "guess_expires_at"}).
			AddRow(photo.ID, "user-2", now, viewExpires, nil))
	mock.ExpectCommit()
	acceptedPhoto, view, err = repo.AcceptChallenge(context.Background(), photo.ID, "user-2", 30*time.Minute, 2*time.Minute, now)
	if err != nil || view.GuessExpiresAt != guessExpires {
		t.Fatalf("legacy re-accept = %+v/%+v, %v", acceptedPhoto, view, err)
	}
}

func TestResultsAndGuessIdempotency(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC().Truncate(time.Microsecond)
	photo := &models.Photo{ID: "photo-1", UserID: "user-1", GroupID: "group-1", StorageKey: "photos/one", MIMEType: "image/jpeg", ByteSize: 10, Lat: 48.8, Long: 2.3, LifecycleStatus: "ready", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(photoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.ID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	gotPhoto, allowed, err := repo.CanViewResults(context.Background(), photo.ID, "user-2", now)
	if err != nil || gotPhoto == nil || allowed {
		t.Fatalf("result visibility = %+v/%v, %v", gotPhoto, allowed, err)
	}
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(photoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	gotPhoto, allowed, err = repo.CanViewResults(context.Background(), photo.ID, "user-1", now)
	if err != nil || !allowed || gotPhoto == nil {
		t.Fatalf("owner result visibility = %+v/%v, %v", gotPhoto, allowed, err)
	}

	if _, err := repo.SubmitGuess(context.Background(), photo.ID, "user-2", 100, 0, now); err != ErrInvalidCoordinate {
		t.Fatalf("invalid guess = %v", err)
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, user_id, group_id.*FOR UPDATE").WithArgs(photo.ID).WillReturnRows(photoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT id, photo_id, user_id").WithArgs(photo.ID, "user-2").WillReturnError(pgx.ErrNoRows)
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-2").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at", "guess_expires_at"}).AddRow(nil, now.Add(-time.Minute), now.Add(time.Hour)))
	mock.ExpectRollback()
	if _, err := repo.SubmitGuess(context.Background(), photo.ID, "user-2", 48.9, 2.4, now); err != ErrViewNotFinished {
		t.Fatalf("guess before delivery confirmation = %v", err)
	}
	guessTime := now.Add(time.Hour)
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, user_id, group_id.*FOR UPDATE").WithArgs(photo.ID).WillReturnRows(photoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT id, photo_id, user_id").WithArgs(photo.ID, "user-2").WillReturnError(pgx.ErrNoRows)
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at", "guess_expires_at"}).AddRow(now.Add(-time.Hour), now.Add(-time.Minute), now.Add(2*time.Hour)))
	// No party is active at the guess instant, so the base score stands.
	mock.ExpectQuery("FROM group_party_times").WithArgs(photo.GroupID, "user-2", guessTime).WillReturnError(pgx.ErrNoRows)
	mock.ExpectExec("INSERT INTO guesses").WithArgs(pgxmock.AnyArg(), photo.ID, "user-2", photo.GroupID, 48.9, 2.4, pgxmock.AnyArg(), pgxmock.AnyArg(), false, guessTime).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	result, err := repo.SubmitGuess(context.Background(), photo.ID, "user-2", 48.9, 2.4, guessTime)
	if err != nil || result == nil || result.Existing || result.Guess.ID == "" {
		t.Fatalf("new guess = %+v, %v", result, err)
	}
	if result.PartyDoubled {
		t.Fatal("a guess without an active party must not be flagged as doubled")
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, user_id, group_id.*FOR UPDATE").WithArgs(photo.ID).WillReturnRows(photoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT id, photo_id, user_id").WithArgs(photo.ID, "user-2").WillReturnRows(guessRows(guessTime))
	mock.ExpectCommit()
	result, err = repo.SubmitGuess(context.Background(), photo.ID, "user-2", 48.9, 2.4, guessTime)
	if err != nil || result == nil || !result.Existing || result.Guess.ID != "guess-1" {
		t.Fatalf("existing guess = %+v, %v", result, err)
	}
}

func TestGuessRejectedAfterGuessWindow(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC().Truncate(time.Microsecond)
	photo := &models.Photo{ID: "photo-1", UserID: "user-1", GroupID: "group-1", StorageKey: "photos/one", MIMEType: "image/jpeg", ByteSize: 10, Lat: 48.8, Long: 2.3, LifecycleStatus: "ready", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}

	// The recorded guess deadline has passed: the server refuses the guess
	// even though the viewing window is closed, so a player who closed the
	// app cannot bypass the deadline by reopening the challenge.
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, user_id, group_id.*FOR UPDATE").WithArgs(photo.ID).WillReturnRows(photoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT id, photo_id, user_id").WithArgs(photo.ID, "user-2").WillReturnError(pgx.ErrNoRows)
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-2").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at", "guess_expires_at"}).AddRow(now.Add(-time.Hour), now.Add(-time.Minute), now.Add(-time.Second)))
	mock.ExpectExec("INSERT INTO guesses").WithArgs(pgxmock.AnyArg(), photo.ID, "user-2", photo.GroupID, 0, 0, 0, 0, true, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	if _, err := repo.SubmitGuess(context.Background(), photo.ID, "user-2", 48.9, 2.4, now); !errors.Is(err, ErrGuessTimeExpired) {
		t.Fatalf("guess after the deadline = %v, want ErrGuessTimeExpired", err)
	}

	// A legacy view row without a recorded deadline (NULL, pre-migration)
	// keeps the previous behavior: no guess-window limit.
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, user_id, group_id.*FOR UPDATE").WithArgs(photo.ID).WillReturnRows(photoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT id, photo_id, user_id").WithArgs(photo.ID, "user-2").WillReturnError(pgx.ErrNoRows)
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-2").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at", "guess_expires_at"}).AddRow(now.Add(-time.Hour), now.Add(-time.Minute), nil))
	mock.ExpectQuery("FROM group_party_times").WithArgs(photo.GroupID, "user-2", now).WillReturnError(pgx.ErrNoRows)
	mock.ExpectExec("INSERT INTO guesses").WithArgs(pgxmock.AnyArg(), photo.ID, "user-2", photo.GroupID, 48.9, 2.4, pgxmock.AnyArg(), pgxmock.AnyArg(), false, now).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	if _, err := repo.SubmitGuess(context.Background(), photo.ID, "user-2", 48.9, 2.4, now); err != nil {
		t.Fatalf("legacy view without a guess deadline must keep guessing, got %v", err)
	}
}

// TestPartyTimeDoubling pins the posting-incentive rule end to end at the
// persistence layer: the same guess scores double exactly when a party
// window is active at the guess instant AND the guesser posted a challenge
// into the group since that window started. A party without a posted
// challenge from the guesser leaves the score untouched.
func TestPartyTimeDoubling(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC().Truncate(time.Microsecond)
	photo := &models.Photo{ID: "photo-1", UserID: "user-1", GroupID: "group-1", StorageKey: "photos/one", MIMEType: "image/jpeg", ByteSize: 10, Lat: 48.8, Long: 2.3, LifecycleStatus: "ready", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}
	guessTime := now.Add(30 * time.Minute)

	submit := func(partyLookup func() *pgxmock.Rows) *GuessResult {
		t.Helper()
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id, user_id, group_id.*FOR UPDATE").WithArgs(photo.ID).WillReturnRows(photoRows(photo))
		mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT id, photo_id, user_id").WithArgs(photo.ID, "user-2").WillReturnError(pgx.ErrNoRows)
		mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-2").
			WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at", "guess_expires_at"}).AddRow(now.Add(-time.Hour), now.Add(-time.Minute), guessTime.Add(time.Minute)))
		if partyLookup != nil {
			mock.ExpectQuery("FROM group_party_times").WithArgs(photo.GroupID, "user-2", guessTime).WillReturnRows(partyLookup())
		} else {
			mock.ExpectQuery("FROM group_party_times").WithArgs(photo.GroupID, "user-2", guessTime).WillReturnError(pgx.ErrNoRows)
		}
		mock.ExpectExec("INSERT INTO guesses").WithArgs(pgxmock.AnyArg(), photo.ID, "user-2", photo.GroupID, 48.9, 2.4, pgxmock.AnyArg(), pgxmock.AnyArg(), false, guessTime).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()
		result, err := repo.SubmitGuess(context.Background(), photo.ID, "user-2", 48.9, 2.4, guessTime)
		if err != nil || result == nil {
			t.Fatalf("submit = %+v, %v", result, err)
		}
		return result
	}

	base := submit(nil)
	if base.PartyDoubled {
		t.Fatal("no active party must not report doubling")
	}
	doubled := submit(func() *pgxmock.Rows {
		return pgxmock.NewRows([]string{"exists"}).AddRow(true)
	})
	if !doubled.PartyDoubled {
		t.Fatal("an eligible party guess must be flagged as doubled")
	}
	if want := 2 * base.Guess.Score; doubled.Guess.Score != want {
		t.Fatalf("doubled score = %d, want %d (base %d)", doubled.Guess.Score, want, base.Guess.Score)
	}
	ineligible := submit(func() *pgxmock.Rows {
		return pgxmock.NewRows([]string{"exists"}).AddRow(false)
	})
	if ineligible.PartyDoubled || ineligible.Guess.Score != base.Guess.Score {
		t.Fatalf("a guesser who did not post during the window must score the base %d, got %d (%v)", base.Guess.Score, ineligible.Guess.Score, ineligible.PartyDoubled)
	}
}

func timeoutReReadRows(now time.Time) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "photo_id", "user_id", "group_id", "lat", "long", "score", "distance", "timed_out", "created_at"}).
		AddRow("timeout-1", "photo-1", "user-2", "group-1", 0.0, 0.0, 0, 0.0, true, now)
}

func TestTimeoutGuessAcceptsClientCountdownSkew(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC().Truncate(time.Microsecond)
	photo := &models.Photo{ID: "photo-1", UserID: "user-1", GroupID: "group-1", StorageKey: "photos/one", MIMEType: "image/jpeg", ByteSize: 10, Lat: 48.8, Long: 2.3, LifecycleStatus: "ready", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}

	// A client countdown fires at its local zero, which can land a tick
	// before the authoritative deadline; the small skew tolerance must let
	// the timeout row be recorded instead of rejecting with 409.
	viewExpires := now.Add(-time.Hour)
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(photoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT id, photo_id, user_id").WithArgs(photo.ID, "user-2").WillReturnError(pgx.ErrNoRows)
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-2").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at", "guess_expires_at"}).
			AddRow(now.Add(-2*time.Hour), viewExpires, now.Add(time.Second)))
	mock.ExpectExec("INSERT INTO guesses").WithArgs(pgxmock.AnyArg(), photo.ID, "user-2", photo.GroupID, 0, 0, 0, 0, true, pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectQuery("SELECT id, photo_id, user_id").WithArgs(photo.ID, "user-2").WillReturnRows(timeoutReReadRows(now))
	result, err := repo.TimeoutGuess(context.Background(), photo.ID, "user-2", now)
	if err != nil || result == nil || !result.Guess.TimedOut {
		t.Fatalf("timeout within skew = %+v, %v", result, err)
	}

	// A call clearly before the deadline is still refused without writing.
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(photoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT id, photo_id, user_id").WithArgs(photo.ID, "user-2").WillReturnError(pgx.ErrNoRows)
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-2").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at", "guess_expires_at"}).
			AddRow(now.Add(-2*time.Hour), viewExpires, now.Add(10*time.Second)))
	if _, err := repo.TimeoutGuess(context.Background(), photo.ID, "user-2", now); !errors.Is(err, ErrViewNotFinished) {
		t.Fatalf("timeout well before deadline = %v, want ErrViewNotFinished", err)
	}
}

func TestLateGuessPersistsTimeoutBeforeExpiryReply(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC().Truncate(time.Microsecond)
	photo := &models.Photo{ID: "photo-1", UserID: "user-1", GroupID: "group-1", StorageKey: "photos/one", MIMEType: "image/jpeg", ByteSize: 10, Lat: 48.8, Long: 2.3, LifecycleStatus: "ready", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}

	// A failed timeout insert surfaces as an error (500 + observability)
	// rather than silently returning 410 without a persisted timeout row.
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id, user_id, group_id.*FOR UPDATE").WithArgs(photo.ID).WillReturnRows(photoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT id, photo_id, user_id").WithArgs(photo.ID, "user-2").WillReturnError(pgx.ErrNoRows)
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-2").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at", "guess_expires_at"}).
			AddRow(now.Add(-time.Hour), now.Add(-time.Minute), now.Add(-time.Second)))
	mock.ExpectExec("INSERT INTO guesses").WithArgs(pgxmock.AnyArg(), photo.ID, "user-2", photo.GroupID, 0, 0, 0, 0, true, pgxmock.AnyArg()).WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()
	_, err := repo.SubmitGuess(context.Background(), photo.ID, "user-2", 48.9, 2.4, now)
	if err == nil || errors.Is(err, ErrGuessTimeExpired) {
		t.Fatalf("failed timeout insert = %v, want a surfaced non-410 error", err)
	}
}

func TestPhotoGuessListsAndErrors(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT g.id, g.photo_id").WithArgs("photo-1").WillReturnRows(pgxmock.NewRows([]string{"id", "photo_id", "user_id", "group_id", "lat", "long", "score", "distance", "timed_out", "created_at", "username", "avatar"}).AddRow("guess-1", "photo-1", "user-2", "group-1", 1.0, 2.0, 80, 20.0, false, now, "alice", "a.png"))
	guesses, err := repo.GuessesForPhoto(context.Background(), "photo-1")
	if err != nil || len(guesses) != 1 || guesses[0].Username != "alice" {
		t.Fatalf("guesses = %+v, %v", guesses, err)
	}
}

func TestViewDeliveryStatus(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").
		WithArgs("photo-1", "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at"}).AddRow(now, now.Add(time.Minute)))
	delivered, expiresAt, err := repo.ViewDeliveryStatus(context.Background(), "photo-1", "user-1")
	if err != nil || !delivered || !expiresAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("delivered view = %v/%v, %v", delivered, expiresAt, err)
	}
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").
		WithArgs("photo-1", "user-2").
		WillReturnError(pgx.ErrNoRows)
	if _, _, err := repo.ViewDeliveryStatus(context.Background(), "photo-1", "user-2"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("missing view error = %v", err)
	}
}
