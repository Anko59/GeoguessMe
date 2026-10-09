package feed

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/pashagolub/pgxmock/v5"
)

func TestTimedResultsWireOmitsPrivateCoordinates(t *testing.T) {
	a, mock := mockAPI(t)
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	a.clock = func() time.Time { return now }
	mock.ExpectQuery("SELECT p.user_id,p.lat,p.long").WithArgs("viewer", testID).WillReturnRows(pgxmock.NewRows([]string{"owner", "lat", "long", "hide_location", "created_at"}).AddRow("author", 48.8, 2.3, true, now))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(testID, "viewer", now).WillReturnRows(pgxmock.NewRows([]string{"allowed"}).AddRow(true))
	mock.ExpectQuery("SELECT g.id,g.user_id,u.username,u.avatar,g.lat,g.long,g.score").WithArgs(testID, "viewer").WillReturnRows(
		pgxmock.NewRows([]string{"id", "user_id", "username", "avatar", "lat", "long", "score", "distance", "timed_out", "created_at", "pin_key", "pin_name", "pin_image"}).AddRow("own", "viewer", "Viewer", "", 1.0, 2.0, 4000, 100.0, false, now, "", "", "").AddRow("peer", "peer", "Peer", "", 3.0, 4.0, 4500, 200.0, false, now, "", "", ""),
	)
	w := httptest.NewRecorder()
	a.TimedResults(w, request(http.MethodGet, ""))
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("response %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "actual_lat") || strings.Contains(w.Body.String(), "actual_long") || !strings.Contains(w.Body.String(), `"location_hidden":true`) {
		t.Fatalf("answer leaked or missing hidden state: %s", w.Body.String())
	}
	var result models.PublicTimedResults
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Guesses) != 2 || result.Guesses[0].Lat == nil || result.Guesses[1].Lat != nil || result.Guesses[1].Long != nil || result.Guesses[1].Distance != nil || result.LocationRevealsAt == nil || !result.LocationRevealsAt.Equal(now.Add(48*time.Hour)) {
		t.Fatalf("private wire data = %+v", result)
	}
}

func TestLegacyGuessWireHonorsLocationPrivacyOnPostAndReload(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodGet} {
		t.Run(method, func(t *testing.T) {
			a, mock := mockAPI(t)
			now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
			a.clock = func() time.Time { return now }
			if method == http.MethodPost {
				mock.ExpectBegin()
				mock.ExpectQuery("SELECT p.user_id,p.lat,p.long.*FOR KEY SHARE").WithArgs("viewer", testID).WillReturnRows(pgxmock.NewRows([]string{"owner", "lat", "long", "hide_location", "created_at"}).AddRow("author", 0.0, 0.0, true, now))
				mock.ExpectExec("INSERT INTO public_guesses").WithArgs(testID, "viewer", 0.0, 0.0, 5000, 0.0).WillReturnResult(pgxmock.NewResult("INSERT", 1))
				mock.ExpectQuery("SELECT score,distance,lat,long").WithArgs(testID, "viewer").WillReturnRows(pgxmock.NewRows([]string{"score", "distance", "lat", "long"}).AddRow(5000, 0.0, 0.0, 0.0))
				mock.ExpectCommit()
			} else {
				mock.ExpectQuery("SELECT g.score,g.distance,g.lat,g.long,p.lat,p.long").WithArgs("viewer", testID).WillReturnRows(pgxmock.NewRows([]string{"score", "distance", "lat", "long", "actual_lat", "actual_long", "owner", "hide_location", "created_at"}).AddRow(5000, 0.0, 0.0, 0.0, 0.0, 0.0, "author", true, now))
			}
			w := httptest.NewRecorder()
			a.Guess(w, request(method, `{"lat":0,"long":0}`))
			if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("response %d: %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "actual_lat") || strings.Contains(w.Body.String(), "actual_long") || !strings.Contains(w.Body.String(), `"location_hidden":true`) || !strings.Contains(w.Body.String(), `"score":5000`) {
				t.Fatalf("legacy privacy response = %s", w.Body.String())
			}
		})
	}
}
