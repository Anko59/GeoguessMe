package feed

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"geoguessme/handlers"
	"geoguessme/internal/models"
	feedrepo "geoguessme/internal/repository/feed"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

const testID = "00000000-0000-0000-0000-000000000001"

func mockAPI(t *testing.T) (*API, pgxmock.PgxPoolIface) {
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
	return NewAPI(feedrepo.NewRepository(mock), nil, nil, nil, time.Now), mock
}

func request(method, body string) *http.Request {
	r := httptest.NewRequest(method, "/", strings.NewReader(body))
	r.SetPathValue("id", testID)
	return r.WithContext(handlers.WithUserID(r.Context(), "viewer"))
}

func TestFeedRejectsInvalidRequestsBeforePersistence(t *testing.T) {
	cases := []struct {
		name, method, body, query string
		handler                   func(*API) http.HandlerFunc
	}{
		{"missing coordinates", "POST", `{}`, "", func(a *API) http.HandlerFunc { return a.Guess }},
		{"latitude missing", "POST", `{"long":0}`, "", func(a *API) http.HandlerFunc { return a.Guess }},
		{"out of range", "POST", `{"lat":91,"long":0}`, "", func(a *API) http.HandlerFunc { return a.Guess }},
		{"unknown field", "POST", `{"lat":0,"long":0,"score":5000}`, "", func(a *API) http.HandlerFunc { return a.Guess }},
		{"blank comment", "POST", `{"content":"  "}`, "", func(a *API) http.HandlerFunc { return a.Comments }},
		{"long comment", "POST", `{"content":"` + strings.Repeat("a", 1001) + `"}`, "", func(a *API) http.HandlerFunc { return a.Comments }},
		{"bad cursor", "GET", "", "cursor=invalid", func(a *API) http.HandlerFunc { return a.List }},
		{"bad limit", "GET", "", "limit=51", func(a *API) http.HandlerFunc { return a.List }},
		{"zero limit", "GET", "", "limit=0", func(a *API) http.HandlerFunc { return a.List }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := mockAPI(t)
			r := request(tc.method, tc.body)
			r.URL.RawQuery = tc.query
			w := httptest.NewRecorder()
			tc.handler(a)(w, r)
			if w.Code != 400 || !strings.Contains(w.Body.String(), `"error"`) {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestFeedMediaOnlyStreamsOriginalForResolvedViewerOrExplicitPlay(t *testing.T) {
	for _, tc := range []struct {
		name              string
		revealed, playing bool
		want              string
	}{
		{"unsolved preview", false, false, "blurred"},
		{"resolved or owner", true, false, "original"},
		{"explicit attempt", false, true, "original"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, mock := mockAPI(t)
			store := &fakeStore{}
			a.store = store
			mock.ExpectQuery("SELECT storage_key,mime_type,preview").WithArgs(testID, "viewer").WillReturnRows(pgxmock.NewRows([]string{"key", "mime", "preview", "revealed"}).AddRow("secret-key", "image/png", []byte("blurred"), tc.revealed))
			w := httptest.NewRecorder()
			a.serveMedia(w, request("GET", ""), tc.playing)
			if w.Code != 200 || w.Body.String() != tc.want {
				t.Fatalf("response %d %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatal("media must not be cached")
			}
			if tc.want == "blurred" && store.reads != 0 {
				t.Fatal("unresolved feed fetched original bytes")
			}
		})
	}
}

type fakeStore struct {
	reads     int
	deleteErr error
	deleted   []string
	putErr    error
}

func (s *fakeStore) Get(context.Context, string) (io.ReadCloser, error) {
	s.reads++
	return io.NopCloser(bytes.NewReader([]byte("original"))), nil
}
func (s *fakeStore) Put(context.Context, string, io.Reader, int64, string) error { return s.putErr }
func (s *fakeStore) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return s.deleteErr
}
func (s *fakeStore) Stat(context.Context, string) (int64, error) { return 8, nil }
func (s *fakeStore) Health(context.Context) error                { return nil }

func TestFeedListContainsNoAnswerAndUsesViewerState(t *testing.T) {
	a, mock := mockAPI(t)
	now := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	columns := []string{"id", "user", "username", "caption", "created_at", "owner", "resolved", "likes", "liked", "comments"}
	mock.ExpectQuery("SELECT p.id, p.user_id").WithArgs("viewer", 21).WillReturnRows(pgxmock.NewRows(columns).AddRow(testID, "author", "Explorer", "Find this place", now, false, false, 3, true, 2))
	w := httptest.NewRecorder()
	a.List(w, request("GET", ""))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for _, secret := range []string{"actual_lat", "storage_key", `"lat"`, `"long"`} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatalf("answer leaked: %s", w.Body.String())
		}
	}
	var page models.PublicFeedPage
	if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Resolved || !page.Items[0].Reacted || page.Items[0].ReactionCount != 3 {
		t.Fatalf("page: %+v", page)
	}
}

func TestFeedErrorsAndCommentOwnership(t *testing.T) {
	a, mock := mockAPI(t)
	mock.ExpectQuery("SELECT p.id, p.user_id").WithArgs("viewer", testID).WillReturnError(pgx.ErrNoRows)
	w := httptest.NewRecorder()
	a.Post(w, request("GET", ""))
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	mock.ExpectExec("DELETE FROM public_comments c USING public_challenges p").WithArgs(testID, testID, "viewer").WillReturnResult(pgxmock.NewResult("DELETE", 0))
	w = httptest.NewRecorder()
	r := request("DELETE", "")
	r.SetPathValue("commentID", testID)
	a.DeleteComment(w, r)
	if w.Code != 404 {
		t.Fatal(w.Code)
	}
	mock.ExpectQuery("WITH challenge AS").WithArgs(testID, "viewer").WillReturnError(errors.New("database unavailable"))
	w = httptest.NewRecorder()
	a.Reaction(w, request("PUT", ""))
	if w.Code != 500 {
		t.Fatal(w.Code)
	}
	if strings.Contains(w.Body.String(), "database unavailable") {
		t.Fatal("database detail exposed")
	}
}

func TestCommentCreatedWithTrimmedContent(t *testing.T) {
	a, mock := mockAPI(t)
	mock.ExpectQuery("WITH inserted AS").WithArgs(pgxmock.AnyArg(), testID, "viewer", "Great place!").WillReturnRows(pgxmock.NewRows([]string{"at", "name"}).AddRow(time.Now(), "Explorer"))
	w := httptest.NewRecorder()
	a.Comments(w, request("POST", `{"content":"  Great place!  "}`))
	if w.Code != 201 || !strings.Contains(w.Body.String(), `"content":"Great place!"`) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
}
