package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"geoguessme/internal/models"
)

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
