package handlers

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"geoguessme/internal/chat"
	"geoguessme/internal/models"
	"geoguessme/internal/storage"

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
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	mock := handlerMock(t)
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
	UploadPhoto(recorder, multipartUploadToGroups(t, []string{groupA, groupB}, false))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("multi-group upload status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"photos"`) || !strings.Contains(recorder.Body.String(), groupB) {
		t.Fatalf("multi-group response missing photos: %s", recorder.Body.String())
	}
}

func TestUploadPhotoHideLocation(t *testing.T) {
	setupHandlers(t)
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	MediaStore = store
	mock := handlerMock(t)
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
	UploadPhoto(recorder, multipartUploadToGroups(t, []string{groupID}, true))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("hide-location upload status = %d (%s)", recorder.Code, recorder.Body.String())
	}
}

func TestChallengeResultsHideLocation(t *testing.T) {
	setupHandlers(t)
	mock := handlerMock(t)
	now := time.Now().UTC()
	groupID := "00000000-0000-0000-0000-000000000001"
	photo := &models.Photo{ID: "00000000-0000-0000-0000-000000000002", UserID: "user-1", GroupID: groupID, StorageKey: "photos/media", MIMEType: "image/png", ByteSize: 4, LifecycleStatus: "ready", HideLocation: true, CreatedAt: now, ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}
	guessesRows := func() *pgxmock.Rows {
		return pgxmock.NewRows([]string{"id", "photo_id", "user_id", "group_id", "lat", "long", "score", "distance", "created_at", "username", "avatar"}).
			AddRow("guess-1", photo.ID, "user-2", groupID, 48.8, 2.3, 80, 10.0, now, "bob", "b.png").
			AddRow("guess-2", photo.ID, "user-3", groupID, 45.7, 4.8, 60, 120.0, now, "carol", "c.png")
	}
	fetch := func(viewerID string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := requestWithUser(http.MethodGet, "/", "", viewerID)
		request.SetPathValue("photoID", photo.ID)
		GetChallengeResults(recorder, request)
		return recorder
	}
	expectResultsQueries := func(viewerID string) {
		mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
		mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, viewerID).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT EXISTS").WithArgs(photo.ID, viewerID).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
		mock.ExpectQuery("SELECT g.id, g.photo_id").WithArgs(photo.ID).WillReturnRows(guessesRows())
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
	// The owner always sees every guess and their own location.
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT g.id, g.photo_id").WithArgs(photo.ID).WillReturnRows(guessesRows())
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
	recorder = fetch("user-2")
	body = recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, "actual_lat") || strings.Contains(body, "location_hidden") || strings.Count(body, `"lat":`) != 2 {
		t.Fatalf("revealed results = %d (%s)", recorder.Code, body)
	}
}

func TestChallengeResultsAndChatRejection(t *testing.T) {
	setupHandlers(t)
	mock := handlerMock(t)
	now := time.Now().UTC()
	groupID := "00000000-0000-0000-0000-000000000001"
	photo := &models.Photo{ID: "00000000-0000-0000-0000-000000000002", UserID: "user-1", GroupID: groupID, StorageKey: "photos/media", MIMEType: "image/png", LifecycleStatus: "ready", CreatedAt: now, ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)}
	mock.ExpectQuery("SELECT id, user_id, group_id").WithArgs(photo.ID).WillReturnRows(handlerPhotoRows(photo))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT g.id, g.photo_id").WithArgs(photo.ID).WillReturnRows(pgxmock.NewRows([]string{"id", "photo_id", "user_id", "group_id", "lat", "long", "score", "distance", "created_at", "username", "avatar"}).AddRow("guess-1", photo.ID, "user-2", groupID, 48.8, 2.3, 80, 10.0, now, "bob", "b.png"))
	recorder := httptest.NewRecorder()
	resultsRequest := requestWithUser(http.MethodGet, "/", "", "user-1")
	resultsRequest.SetPathValue("photoID", photo.ID)
	GetChallengeResults(recorder, resultsRequest)
	if recorder.Code != http.StatusOK {
		t.Fatalf("results status = %d", recorder.Code)
	}

	RuntimeConfig.AllowedOrigins = []string{"http://allowed.test"}
	HubInstance = chat.NewHub(nil, nil)
	badOrigin := requestWithUser(http.MethodGet, "/?group_id="+groupID+"&ticket=t", "", "user-1")
	badOrigin.Header.Set("Origin", "http://evil.test")
	recorder = httptest.NewRecorder()
	HandleChat(recorder, badOrigin)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("bad origin status = %d", recorder.Code)
	}
	HubInstance = nil
}
