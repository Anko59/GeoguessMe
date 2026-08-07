package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/pashagolub/pgxmock/v4"
)

func profileUser(id, username string) *models.User {
	now := time.Now().UTC()
	return &models.User{ID: id, Username: username, Email: username + "@example.test", Avatar: "avatar.png", CreatedAt: now, UpdatedAt: now}
}

func TestGetPublicProfile(t *testing.T) {
	setupHandlers(t)
	mock := handlerMock(t)
	target := profileUser("user-2", "bob")
	viewer := profileUser("user-1", "alice")

	// Unsupported methods are rejected before any lookup.
	requireStatus(t, GetPublicProfile, requestWithUser(http.MethodPost, "/", "", viewer.ID), http.StatusMethodNotAllowed)

	// Unknown target player.
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(target.ID).WillReturnRows(pgxmock.NewRows([]string{"id", "username", "email", "password", "avatar", "verified", "auth_version", "created_at", "updated_at"}))
	requireStatus(t, GetPublicProfile, getPublicProfileRequest(viewer.ID, target.ID), http.StatusNotFound)

	// Players without a shared group cannot view each other's profile.
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(target.ID).WillReturnRows(handlerUserRows(target))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(target.ID, viewer.ID).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	requireStatus(t, GetPublicProfile, getPublicProfileRequest(viewer.ID, target.ID), http.StatusForbidden)

	// Players sharing a group see the target's progression without email.
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(target.ID).WillReturnRows(handlerUserRows(target))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(target.ID, viewer.ID).WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(score\\), 0\\), COUNT\\(\\*\\)").WithArgs(target.ID).
		WillReturnRows(pgxmock.NewRows([]string{"total_points", "guess_count"}).AddRow(int64(7600), int64(3)))
	mock.ExpectQuery("WITH totals AS").WithArgs(target.ID).WillReturnRows(pgxmock.NewRows([]string{"rank", "total_players"}).AddRow(int64(3), int64(1943)))
	recorder := httptest.NewRecorder()
	GetPublicProfile(recorder, getPublicProfileRequest(viewer.ID, target.ID))
	if recorder.Code != http.StatusOK {
		t.Fatalf("public profile status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{`"username":"bob"`, `"total_points":7600`, `"name":"Lost Tourist"`, `"global_rank":{"rank":3,"total_players":1943}`} {
		if !strings.Contains(body, want) {
			t.Fatalf("public profile missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, "email") {
		t.Fatalf("public profile must not expose email: %s", body)
	}

	// Viewing yourself skips the shared-group check.
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(viewer.ID).WillReturnRows(handlerUserRows(viewer))
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(score\\), 0\\), COUNT\\(\\*\\)").WithArgs(viewer.ID).
		WillReturnRows(pgxmock.NewRows([]string{"total_points", "guess_count"}).AddRow(int64(0), int64(0)))
	mock.ExpectQuery("WITH totals AS").WithArgs(viewer.ID).WillReturnRows(pgxmock.NewRows([]string{"rank", "total_players"}).AddRow(int64(0), int64(0)))
	requireStatus(t, GetPublicProfile, getPublicProfileRequest(viewer.ID, viewer.ID), http.StatusOK)
}

func getPublicProfileRequest(viewerID, targetID string) *http.Request {
	request := requestWithUser(http.MethodGet, "/", "", viewerID)
	request.SetPathValue("userID", targetID)
	return request
}
