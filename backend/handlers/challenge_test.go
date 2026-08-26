package handlers

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"geoguessme/internal/chat"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	"geoguessme/internal/storage"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func multipartUploadToGroups(t *testing.T, groupIDs []string, hideLocation bool) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("photo", "test.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(mustDecodeBase64("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")); err != nil {
		t.Fatal(err)
	}
	for _, groupID := range groupIDs {
		if err := writer.WriteField("group_ids", groupID); err != nil {
			t.Fatal(err)
		}
	}
	if hideLocation {
		if err := writer.WriteField("hide_location", "true"); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.WriteField("lat", "48.8566"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("long", "2.3522"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := requestWithUser(http.MethodPost, "/", "", "user-1")
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Body = io.NopCloser(body)
	return request
}

func TestUploadPhotoToMultipleGroups(t *testing.T) {
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mock := newMockPool(t)
	repos := repository.NewRepository(mock)
	gameAPI := NewGameAPI(repos.Groups, repos.Chat, repos, store, handlerConfig(), nil, nil, time.Now)
	groupA := "00000000-0000-0000-0000-000000000001"
	groupB := "00000000-0000-0000-0000-000000000002"
	for _, groupID := range []string{groupA, groupB} {
		mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	}
	mock.ExpectBegin()
	insertArgs := func() []any {
		args := make([]any, 0, 14)
		for i := 0; i < 10; i++ {
			args = append(args, pgxmock.AnyArg())
		}
		args = append(args, false, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg())
		return args
	}
	mock.ExpectExec("INSERT INTO photos").WithArgs(insertArgs()...).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO photos").WithArgs(insertArgs()...).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	recorder := httptest.NewRecorder()
	gameAPI.UploadPhoto(recorder, multipartUploadToGroups(t, []string{groupA, groupB}, false))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("multi-group upload status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"photos"`) || !strings.Contains(recorder.Body.String(), groupB) {
		t.Fatalf("multi-group response missing photos: %s", recorder.Body.String())
	}
}

func TestUploadPhotoHideLocation(t *testing.T) {
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mock := newMockPool(t)
	repos := repository.NewRepository(mock)
	gameAPI := NewGameAPI(repos.Groups, repos.Chat, repos, store, handlerConfig(), nil, nil, time.Now)
	groupID := "00000000-0000-0000-0000-000000000001"
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectBegin()
	args := make([]any, 0, 14)
	for i := 0; i < 10; i++ {
		args = append(args, pgxmock.AnyArg())
	}
	args = append(args, true, pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg())
	mock.ExpectExec("INSERT INTO photos").WithArgs(args...).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	recorder := httptest.NewRecorder()
	gameAPI.UploadPhoto(recorder, multipartUploadToGroups(t, []string{groupID}, true))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("hide-location upload status = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

func TestChallengeResultsHideLocation(t *testing.T) {
	mock := newMockPool(t)
	gameAPI := newGameAPI(t, mock)
	now := time.Now().UTC()
	groupID := "00000000-0000-0000-0000-000000000001"
	photo := &models.Photo{ID: "00000000-0000-0000-0000-000000000002", UserID: "user-1", GroupID: groupID, StorageKey: "photos/media", MIMEType: "image/png", ByteSize: 4, LifecycleStatus: "ready", HideLocation: true, CreatedAt: now, ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}
	guessesRows := func() *pgxmock.Rows {
		return pgxmock.NewRows([]string{"id", "photo_id", "user_id", "group_id", "lat", "long", "score", "distance", "created_at", "username", "avatar"}).
			AddRow("guess-1", photo.ID, "user-2", groupID, 48.8, 2.3, 80, 10.0, now, "bob", "b.png").
			AddRow("guess-2", photo.ID, "user-3", groupID, 45.7, 4.8, 60, 120.0, now, "carol", "c.png")
	}
	challengesRows := func() *pgxmock.Rows {
		return pgxmock.NewRows([]string{"id", "created_at", "user_id", "score"}).
			AddRow(photo.ID, now, "user-2", 80).
			AddRow(photo.ID, now, "user-3", 60)
	}
	fetch := func(viewerID string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := requestWithUser(http.MethodGet, "/", "", viewerID)
		request.SetPathValue("photoID", photo.ID)
		gameAPI.GetChallengeResults(recorder, request)
		return recorder
	}
	expectResultsQueries := func(viewerID string) {
		mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
		mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, viewerID).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.ID, viewerID).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT g.id, g.photo_id").WithArgs(photo.ID).WillReturnRows(guessesRows())
		mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at.*AND p\.created_at >= \$1 ORDER BY`).WithArgs(pgxmock.AnyArg()).WillReturnRows(challengesRows())
	}
	// A guesser sees scores, their own guessed point and distance, but not the
	// actual location nor the other players' guessed points while it is hidden.
	expectResultsQueries("user-2")
	recorder := fetch("user-2")
	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || strings.Contains(body, "actual_lat") || !strings.Contains(body, `"location_hidden":true`) {
		t.Fatalf("hidden results = %d (%s)", recorder.Code, body)
	}
	if strings.Count(body, `"lat":`) != 1 {
		t.Fatalf("hidden results must return only the viewer's guessed point, got %s", body)
	}
	if strings.Contains(body, `"distance":10`) && strings.Contains(body, `"username":"carol"`) && strings.Index(body, `"distance":10`) > strings.Index(body, `"username":"carol"`) {
		t.Fatalf("carol's distance must be hidden, got %s", body)
	}
	if !strings.Contains(body, `"distance":10`) {
		t.Fatalf("viewer's own guess distance must be present, got %s", body)
	}
	if !strings.Contains(body, `"elo_delta":20`) || !strings.Contains(body, `"elo_delta":-20`) {
		t.Fatalf("results must include computed weekly elo_delta, got %s", body)
	}
	// The owner always sees every guess and their own location.
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT g.id, g.photo_id").WithArgs(photo.ID).WillReturnRows(guessesRows())
	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at.*AND p\.created_at >= \$1 ORDER BY`).WithArgs(pgxmock.AnyArg()).WillReturnRows(challengesRows())
	recorder = fetch("user-1")
	body = recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "actual_lat") || strings.Count(body, `"lat":`) != 2 {
		t.Fatalf("owner results = %d (%s)", recorder.Code, body)
	}
	// After the hide duration the location is revealed to everyone.
	revealed := *photo
	revealed.CreatedAt = now.Add(-49 * time.Hour)
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(&revealed))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.ID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT g.id, g.photo_id").WithArgs(photo.ID).WillReturnRows(guessesRows())
	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at.*AND p\.created_at >= \$1 ORDER BY`).WithArgs(pgxmock.AnyArg()).WillReturnRows(challengesRows())
	recorder = fetch("user-2")
	body = recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "actual_lat") || strings.Contains(body, "location_hidden") || strings.Count(body, `"lat":`) != 2 {
		t.Fatalf("revealed results = %d (%s)", recorder.Code, body)
	}
}

func TestChallengeResultsAndChatRejection(t *testing.T) {
	mock := newMockPool(t)
	gameAPI := newGameAPI(t, mock)
	now := time.Now().UTC()
	groupID := "00000000-0000-0000-0000-000000000001"
	photo := &models.Photo{ID: "00000000-0000-0000-0000-000000000002", UserID: "user-1", GroupID: groupID, StorageKey: "photos/media", MIMEType: "image/png", LifecycleStatus: "ready", CreatedAt: now, ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT g.id, g.photo_id").WithArgs(photo.ID).WillReturnRows(pgxmock.NewRows([]string{"id", "photo_id", "user_id", "group_id", "lat", "long", "score", "distance", "created_at", "username", "avatar"}).AddRow("guess-1", photo.ID, "user-2", groupID, 48.8, 2.3, 80, 10.0, now, "bob", "b.png"))
	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at.*AND p\.created_at >= \$1 ORDER BY`).WithArgs(pgxmock.AnyArg()).WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "user_id", "score"}))
	recorder := httptest.NewRecorder()
	resultsRequest := requestWithUser(http.MethodGet, "/", "", "user-1")
	resultsRequest.SetPathValue("photoID", photo.ID)
	gameAPI.GetChallengeResults(recorder, resultsRequest)
	if recorder.Code != http.StatusOK {
		t.Fatalf("results status = %d", recorder.Code)
	}

	hub := chat.NewHub(nil, nil)
	defer hub.Stop()
	chatAPI := newChatAPI(t, mock, mustTestStore(t), hub)
	badOrigin := requestWithUser(http.MethodGet, "/?group_id="+groupID+"&ticket=t", "", "user-1")
	badOrigin.Header.Set("Origin", "http://evil.test")
	recorder = httptest.NewRecorder()
	chatAPI.HandleChat(recorder, badOrigin)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("bad origin status = %d", recorder.Code)
	}
}

// TestChallengeMediaViewWindow pins the challenge viewing-window contract:
// media is served while it has never been fully delivered, the window starts
// at the first full delivery, and a re-fetch after the window is always
// denied (view-once).
func TestChallengeMediaViewWindow(t *testing.T) {
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mock := newMockPool(t)
	repos := repository.NewRepository(mock)
	gameAPI := NewGameAPI(repos.Groups, repos.Chat, repos, store, handlerConfig(), nil, nil, time.Now)
	now := time.Now().UTC().Truncate(time.Microsecond)
	photo := &models.Photo{ID: "00000000-0000-0000-0000-000000000002", UserID: "user-2", GroupID: "00000000-0000-0000-0000-000000000001", StorageKey: "photos/media", MIMEType: "image/png", ByteSize: 4, Lat: 48.8, Long: 2.3, LifecycleStatus: "ready", CreatedAt: now, ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}
	if err := store.Put(context.Background(), photo.StorageKey, bytes.NewReader([]byte("data")), 4, photo.MIMEType); err != nil {
		t.Fatal(err)
	}
	serve := func(t *testing.T) *httptest.ResponseRecorder {
		t.Helper()
		recorder := httptest.NewRecorder()
		request := requestWithUser(http.MethodGet, "/", "", "user-1")
		request.SetPathValue("photoID", photo.ID)
		gameAPI.ServeChallengeMedia(recorder, request)
		return recorder
	}

	// A player who never received the media can still fetch it after the
	// original accept window, as long as the challenge is still live.
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at"}).AddRow(nil, now.Add(-time.Minute)))
	mock.ExpectQuery("UPDATE challenge_views").WithArgs(photo.ID, "user-1", pgxmock.AnyArg(), int64(handlerConfig().ViewWindow.Seconds()), int64(handlerConfig().GuessWindow.Seconds())).
		WillReturnRows(pgxmock.NewRows([]string{"view_expires_at", "guess_expires_at"}).AddRow(now.Add(handlerConfig().ViewWindow), now.Add(handlerConfig().ViewWindow+handlerConfig().GuessWindow)))
	if recorder := serve(t); recorder.Code != http.StatusOK {
		t.Fatalf("never-delivered media = %d (%s)", recorder.Code, recorder.Body.String())
	}

	// A re-fetch after the window has closed is denied.
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at"}).AddRow(now.Add(-time.Minute), now.Add(-time.Minute)))
	if recorder := serve(t); recorder.Code != http.StatusForbidden {
		t.Fatalf("expired re-fetch = %d (%s)", recorder.Code, recorder.Body.String())
	}

	// Within the delivered window the media is served again; the window is
	// still not extended for a second delivery.
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT media_delivered_at, view_expires_at").WithArgs(photo.ID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at"}).AddRow(now.Add(-time.Minute), now.Add(time.Hour)))
	mock.ExpectQuery("UPDATE challenge_views").WithArgs(photo.ID, "user-1", pgxmock.AnyArg(), int64(handlerConfig().ViewWindow.Seconds()), int64(handlerConfig().GuessWindow.Seconds())).
		WillReturnRows(pgxmock.NewRows([]string{"view_expires_at", "guess_expires_at"}).AddRow(now.Add(time.Hour), now.Add(time.Hour+handlerConfig().GuessWindow)))
	if recorder := serve(t); recorder.Code != http.StatusOK {
		t.Fatalf("within-window re-fetch = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

func TestConfirmChallengeMediaDeliveredReturnsAuthoritativeDeadline(t *testing.T) {
	mock := newMockPool(t)
	gameAPI := newGameAPI(t, mock)
	photoID := "00000000-0000-0000-0000-000000000002"
	expiresAt := time.Now().UTC().Add(handlerConfig().ViewWindow)
	guessExpiresAt := expiresAt.Add(handlerConfig().GuessWindow)
	mock.ExpectQuery("UPDATE challenge_views").
		WithArgs(photoID, "user-1", pgxmock.AnyArg(), int64(handlerConfig().ViewWindow.Seconds()), int64(handlerConfig().GuessWindow.Seconds())).
		WillReturnRows(pgxmock.NewRows([]string{"view_expires_at", "guess_expires_at"}).AddRow(expiresAt, guessExpiresAt))
	request := requestWithUser(http.MethodPost, "/", "", "user-1")
	request.SetPathValue("photoID", photoID)
	recorder := httptest.NewRecorder()
	gameAPI.ConfirmChallengeMediaDelivered(recorder, request)
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte("view_expires_at")) || !bytes.Contains(recorder.Body.Bytes(), []byte("guess_expires_at")) {
		t.Fatalf("delivery confirmation = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

// guessPhotoRow builds the photo row the challenge guess path reads under a
// row lock, in the canonical scanPhoto column order.
func guessPhotoRow(photo *models.Photo) *pgxmock.Rows {
	return pgxmock.NewRows([]string{"id", "user_id", "group_id", "url", "storage_key", "mime_type", "byte_size", "lat", "long", "lifecycle_status", "hide_location", "created_at", "expires_at", "retention_at"}).
		AddRow(photo.ID, photo.UserID, photo.GroupID, photo.URL, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.Lat, photo.Long, photo.LifecycleStatus, photo.HideLocation, photo.CreatedAt, photo.ExpiresAt, photo.RetentionAt)
}

// TestSubmitChallengeGuessBranches exercises every documented response of the
// guess endpoint: coordinate validation, the membership/ownership/expiry/view
// gates enforced by the persistence layer, and the idempotent duplicate path.
func TestSubmitChallengeGuessBranches(t *testing.T) {
	photoID := "00000000-0000-0000-0000-000000000002"
	now := time.Now().UTC()
	photo := &models.Photo{ID: photoID, UserID: "user-2", GroupID: "00000000-0000-0000-0000-000000000001", Lat: 48.8, Long: 2.3, LifecycleStatus: "ready", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(2 * time.Hour), RetentionAt: now.Add(24 * time.Hour)}
	guessRequest := func(body string) *http.Request {
		request := requestWithUser(http.MethodPost, "/", body, "user-1")
		request.SetPathValue("photoID", photoID)
		return request
	}

	t.Run("invalid coordinates rejected before persistence", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		requireStatus(t, gameAPI.SubmitChallengeGuess, guessRequest(`{"lat":200,"long":0}`), http.StatusBadRequest)
	})

	t.Run("missing challenge is 404", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photoID).
			WillReturnRows(pgxmock.NewRows([]string{"id"}))
		mock.ExpectRollback()
		requireStatus(t, gameAPI.SubmitChallengeGuess, guessRequest(`{"lat":48.8,"long":2.3}`), http.StatusNotFound)
	})

	t.Run("non-member is 403", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photoID).WillReturnRows(guessPhotoRow(photo))
		mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectRollback()
		requireStatus(t, gameAPI.SubmitChallengeGuess, guessRequest(`{"lat":48.8,"long":2.3}`), http.StatusForbidden)
	})

	t.Run("own challenge is 403", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		ownPhoto := *photo
		ownPhoto.UserID = "user-1"
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photoID).WillReturnRows(guessPhotoRow(&ownPhoto))
		mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectRollback()
		requireStatus(t, gameAPI.SubmitChallengeGuess, guessRequest(`{"lat":48.8,"long":2.3}`), http.StatusForbidden)
	})

	t.Run("expired challenge is 410", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		expired := *photo
		expired.ExpiresAt = now.Add(-time.Minute)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photoID).WillReturnRows(guessPhotoRow(&expired))
		mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT id, photo_id, user_id, group_id, lat, long, score, distance, created_at FROM guesses").
			WithArgs(photoID, "user-1").WillReturnError(pgx.ErrNoRows)
		mock.ExpectRollback()
		requireStatus(t, gameAPI.SubmitChallengeGuess, guessRequest(`{"lat":48.8,"long":2.3}`), http.StatusGone)
	})

	t.Run("never-viewed challenge is 403", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photoID).WillReturnRows(guessPhotoRow(photo))
		mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT id, photo_id, user_id, group_id, lat, long, score, distance, created_at FROM guesses").
			WithArgs(photoID, "user-1").WillReturnError(pgx.ErrNoRows)
		mock.ExpectQuery("SELECT media_delivered_at, view_expires_at, guess_expires_at FROM challenge_views").
			WithArgs(photoID, "user-1").WillReturnError(pgx.ErrNoRows)
		mock.ExpectRollback()
		requireStatus(t, gameAPI.SubmitChallengeGuess, guessRequest(`{"lat":48.8,"long":2.3}`), http.StatusForbidden)
	})

	t.Run("open viewing window is 409", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photoID).WillReturnRows(guessPhotoRow(photo))
		mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT id, photo_id, user_id, group_id, lat, long, score, distance, created_at FROM guesses").
			WithArgs(photoID, "user-1").WillReturnError(pgx.ErrNoRows)
		mock.ExpectQuery("SELECT media_delivered_at, view_expires_at, guess_expires_at FROM challenge_views").
			WithArgs(photoID, "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at", "guess_expires_at"}).
				AddRow(time.Now(), now.Add(time.Hour), now.Add(2*time.Hour)))
		mock.ExpectRollback()
		requireStatus(t, gameAPI.SubmitChallengeGuess, guessRequest(`{"lat":48.8,"long":2.3}`), http.StatusConflict)
	})

	t.Run("guess window expired is 410", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photoID).WillReturnRows(guessPhotoRow(photo))
		mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT id, photo_id, user_id, group_id, lat, long, score, distance, created_at FROM guesses").
			WithArgs(photoID, "user-1").WillReturnError(pgx.ErrNoRows)
		mock.ExpectQuery("SELECT media_delivered_at, view_expires_at, guess_expires_at FROM challenge_views").
			WithArgs(photoID, "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"media_delivered_at", "view_expires_at", "guess_expires_at"}).
				AddRow(now.Add(-time.Hour), now.Add(-time.Minute), now.Add(-time.Second)))
		mock.ExpectRollback()
		recorder := httptest.NewRecorder()
		gameAPI.SubmitChallengeGuess(recorder, guessRequest(`{"lat":48.8,"long":2.3}`))
		if recorder.Code != http.StatusGone {
			t.Fatalf("guess after deadline status = %d (%s)", recorder.Code, recorder.Body.String())
		}
		if !strings.Contains(recorder.Body.String(), "guess_time_expired") {
			t.Fatalf("guess after deadline body missing error code: %s", recorder.Body.String())
		}
	})

	t.Run("duplicate guess returns 200 without a new broadcast", func(t *testing.T) {
		mock := newMockPool(t)
		gameAPI := newGameAPI(t, mock)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photoID).WillReturnRows(guessPhotoRow(photo))
		mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.GroupID, "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT id, photo_id, user_id, group_id, lat, long, score, distance, created_at FROM guesses").
			WithArgs(photoID, "user-1").
			WillReturnRows(pgxmock.NewRows([]string{"id", "photo_id", "user_id", "group_id", "lat", "long", "score", "distance", "created_at"}).
				AddRow("guess-1", photoID, "user-1", photo.GroupID, 48.8, 2.3, 4000, 100, now))
		mock.ExpectCommit()
		requireStatus(t, gameAPI.SubmitChallengeGuess, guessRequest(`{"lat":48.8,"long":2.3}`), http.StatusOK)
	})
}

func TestReactionEmojiAliasIsRejected(t *testing.T) {
	// The deprecated emoji request-field alias was removed (PR 12): a reaction
	// sent via the emoji field is no longer parsed, so the request is rejected
	// instead of being mapped onto the reaction key. Only the reaction field
	// is accepted by the contract.
	mock := newMockPool(t)
	chatAPI := newChatAPI(t, mock, nil, nil)
	messageID := "00000000-0000-0000-0000-000000000002"
	request := requestWithUser(http.MethodPut, "/", `{"emoji":"👍"}`, "user-1")
	request.SetPathValue("messageID", messageID)
	requireStatus(t, chatAPI.SetMessageReaction, request, http.StatusBadRequest)
}
