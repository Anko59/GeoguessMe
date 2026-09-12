package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/pashagolub/pgxmock/v4"
)

func TestGroupChallengeFeed(t *testing.T) {
	const groupID = "00000000-0000-0000-0000-000000000001"
	for _, tc := range []struct {
		name, target string
		member       bool
		status       int
	}{
		{"invalid group", "/?group_id=invalid", false, 400},
		{"missing group", "/", false, 400},
		{"outsider", "/?group_id=" + groupID, false, 403},
		{"invalid cursor", "/?group_id=" + groupID + "&cursor=invalid", true, 400},
		{"empty group", "/?group_id=" + groupID, true, 200},
		{"database failure", "/?group_id=" + groupID, true, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := newMockPool(t)
			api := newGameAPI(t, pool)
			if tc.name != "invalid group" && tc.name != "missing group" {
				pool.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "viewer").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(tc.member))
			}
			if tc.status == 200 {
				pool.ExpectQuery("SELECT p.id").WithArgs(groupID, "viewer").WillReturnRows(pgxmock.NewRows([]string{"id", "group_id", "user_id", "username", "created_at", "expires_at", "lat", "long", "hide_location", "guessed"}))
			}
			if tc.status == 500 {
				pool.ExpectQuery("SELECT p.id").WithArgs(groupID, "viewer").WillReturnError(errors.New("private database details"))
			}
			recorder := httptest.NewRecorder()
			api.GetGroupChallenges(recorder, requestWithUser(http.MethodGet, tc.target, "", "viewer"))
			if recorder.Code != tc.status {
				t.Fatalf("response: %d %s", recorder.Code, recorder.Body.String())
			}
			if tc.status == 200 && (!strings.Contains(recorder.Body.String(), `"items":[]`) || recorder.Header().Get("Cache-Control") != "private, no-store") {
				t.Fatalf("invalid empty response: %s", recorder.Body.String())
			}
			if tc.status == 500 && strings.Contains(recorder.Body.String(), "private database details") {
				t.Fatal("database details leaked in the response")
			}
		})
	}
	pool := newMockPool(t)
	api := newGameAPI(t, pool)
	requireStatus(t, api.GetGroupChallenges, requestWithUser(http.MethodPost, "/", "", "viewer"), 405)
	pool.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "viewer").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	pool.ExpectQuery("SELECT p.id").WithArgs(groupID, "viewer").WillReturnRows(pgxmock.NewRows([]string{"id", "group_id", "user_id", "username", "created_at", "expires_at", "lat", "long", "hide_location", "guessed"}).AddRow("photo", groupID, "poster", "Alice", time.Now(), time.Now().Add(time.Hour), 48.0, 2.0, false, false))
	recorder := httptest.NewRecorder()
	api.GetGroupChallenges(recorder, requestWithUser(http.MethodGet, "/?group_id="+groupID, "", "viewer"))
	if recorder.Code != 200 || strings.Contains(recorder.Body.String(), `"lat"`) || strings.Contains(recorder.Body.String(), `"long"`) {
		t.Fatalf("unplayed coordinates leaked: %s", recorder.Body.String())
	}
}

// stubGroupReader is the fake persistence boundary for the migrated group read
// handlers. It lets handler tests exercise GetUserGroups without swapping the
// database.DB package global.
type stubGroupReader struct {
	groups []models.Group
	err    error
}

func (s stubGroupReader) UserGroups(ctx context.Context, userID string) ([]models.Group, error) {
	return s.groups, s.err
}

func TestGetUserGroupsReturnsReaderGroups(t *testing.T) {
	api := NewGroupAPI(stubGroupReader{groups: []models.Group{
		{ID: "g1", Name: "Paris", Code: "ABC123"},
	}})
	recorder := httptest.NewRecorder()
	api.GetUserGroups(recorder, requestWithUser(http.MethodGet, "/", "", "user-1"))

	if recorder.Code != http.StatusOK {
		t.Fatalf("user groups status = %d, want 200 (%s)", recorder.Code, recorder.Body.String())
	}
	var groups []models.Group
	if err := decodeJSONBody(recorder, &groups); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != "g1" || groups[0].Name != "Paris" {
		t.Fatalf("groups = %+v", groups)
	}
}

func TestGetUserGroupsKeepsNullResponseForEmptyGroups(t *testing.T) {
	// The pre-migration handler encoded a nil slice as `null`; the migrated
	// handler must preserve the wire shape.
	api := NewGroupAPI(stubGroupReader{})
	recorder := httptest.NewRecorder()
	api.GetUserGroups(recorder, requestWithUser(http.MethodGet, "/", "", "user-1"))

	if recorder.Code != http.StatusOK {
		t.Fatalf("user groups status = %d, want 200", recorder.Code)
	}
	if body := strings.TrimSpace(recorder.Body.String()); body != "null" {
		t.Fatalf("empty groups body = %q, want null", body)
	}
}

func TestGetUserGroupsRejectsUnsupportedMethods(t *testing.T) {
	api := NewGroupAPI(stubGroupReader{})
	recorder := httptest.NewRecorder()
	api.GetUserGroups(recorder, requestWithUser(http.MethodPatch, "/", `{}`, "user-1"))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("user groups PATCH status = %d, want 405", recorder.Code)
	}
}

func TestGetUserGroupsReaderErrorUsesErrorEnvelope(t *testing.T) {
	api := NewGroupAPI(stubGroupReader{err: errors.New("database unavailable")})
	recorder := httptest.NewRecorder()
	api.GetUserGroups(recorder, requestWithUser(http.MethodGet, "/", "", "user-1"))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("user groups error status = %d, want 500", recorder.Code)
	}
	var envelope errorEnvelope
	if err := decodeJSONBody(recorder, &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error.Code != "internal_error" || envelope.Error.Message != "Unable to load groups" {
		t.Fatalf("error envelope = %+v", envelope)
	}
}

func decodeJSONBody(recorder *httptest.ResponseRecorder, target any) error {
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		return errors.New("missing application/json content type")
	}
	return json.NewDecoder(recorder.Body).Decode(target)
}
