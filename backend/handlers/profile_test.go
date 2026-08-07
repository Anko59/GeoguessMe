package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"geoguessme/internal/auth"
	"geoguessme/internal/config"
	"geoguessme/internal/database"
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
	expectProfileQueries(t, mock, target.ID, 7600, 3, 2533.33, 3, 1943, 7, 1943)
	recorder := httptest.NewRecorder()
	GetPublicProfile(recorder, getPublicProfileRequest(viewer.ID, target.ID))
	if recorder.Code != http.StatusOK {
		t.Fatalf("public profile status = %d (%s)", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	for _, want := range []string{`"username":"bob"`, `"total_points":7600`, `"name":"Lost Tourist"`, `"global_rank":{"rank":3,"total_players":1943}`, `"average_score":2533.33`, `"global_average_rank":{"rank":7,"total_players":1943}`, `"global_elo_rank":{"rank":0,"total_players":0}`} {
		if !strings.Contains(body, want) {
			t.Fatalf("public profile missing %s: %s", want, body)
		}
	}
	if strings.Contains(body, "email") {
		t.Fatalf("public profile must not expose email: %s", body)
	}

	// Viewing yourself skips the shared-group check.
	mock.ExpectQuery("SELECT .*FROM users WHERE id").WithArgs(viewer.ID).WillReturnRows(handlerUserRows(viewer))
	expectProfileQueries(t, mock, viewer.ID, 0, 0, 0, 0, 0, 0, 0)
	requireStatus(t, GetPublicProfile, getPublicProfileRequest(viewer.ID, viewer.ID), http.StatusOK)
}

func getPublicProfileRequest(viewerID, targetID string) *http.Request {
	request := requestWithUser(http.MethodGet, "/", "", viewerID)
	request.SetPathValue("userID", targetID)
	return request
}

// validationStore is a no-op media store so handler tests can exercise the
// upload path without a real object store.
type validationStore struct{}

func (validationStore) Put(context.Context, string, io.Reader, int64, string) error { return nil }
func (validationStore) Delete(context.Context, string) error                        { return nil }
func (validationStore) Get(context.Context, string) (io.ReadCloser, error)          { return nil, nil }
func (validationStore) Stat(context.Context, string) (int64, error)                 { return 0, nil }
func (validationStore) Health(context.Context) error                                { return nil }

func handlerMock(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB
	database.DB = mock
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
		database.DB = previous
	})
	return mock
}

// expectProfileQueries queues every SQL expectation a profile fetch issues:
// score stats (sum, count, average), the lifetime-points global rank, the
// average global rank, and the global Elo challenge load.
func expectProfileQueries(t *testing.T, mock pgxmock.PgxPoolIface, userID string, totalPoints, guessCount int64, average float64, pointsRank, pointsPlayers, averageRank, averagePlayers int64) {
	t.Helper()
	mock.ExpectQuery("SELECT COALESCE\\(SUM\\(score\\), 0\\), COUNT\\(\\*\\), COALESCE\\(AVG\\(score\\), 0\\)").WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"total_points", "guess_count", "average_score"}).AddRow(totalPoints, guessCount, average))
	mock.ExpectQuery("WITH totals AS").WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"rank", "total_players"}).AddRow(pointsRank, pointsPlayers))
	mock.ExpectQuery("WITH scores AS").WithArgs(userID).
		WillReturnRows(pgxmock.NewRows([]string{"rank", "total_players"}).AddRow(averageRank, averagePlayers))
	mock.ExpectQuery(`(?s)SELECT p\.id, p\.created_at, g\.user_id, g\.score.*WHERE TRUE ORDER BY`).
		WillReturnRows(pgxmock.NewRows([]string{"id", "created_at", "user_id", "score"}))
}

func handlerConfig() *config.Config {
	return &config.Config{
		Environment: "test", PublicURL: "http://localhost:8080", JWTSecret: "test_secret_key_at_least_32_characters_long",
		AccessTokenTTL: 15 * time.Minute, RefreshTokenTTL: 24 * time.Hour, VerificationTTL: 24 * time.Hour, ResetTTL: time.Hour,
		PasswordHashCost: 4, UploadMaxBytes: 5 * 1024 * 1024, AvatarMaxBytes: 25 * 1024 * 1024, UploadMaxPixels: 100000, ChallengeTTL: time.Hour,
		ViewWindow: time.Minute, LocationHide: 48 * time.Hour, PhotoRetention: 24 * time.Hour, AllowedOrigins: []string{"http://localhost:8080"},
	}
}

func setupHandlers(t *testing.T) {
	t.Helper()
	cfg := handlerConfig()
	Configure(cfg, nil, nil)
	auth.Init(cfg.JWTSecret)
}
