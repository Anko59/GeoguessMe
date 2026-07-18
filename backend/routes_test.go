package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pashagolub/pgxmock/v4"

	"geoguessme/internal/config"
	"geoguessme/internal/database"
	"geoguessme/internal/middleware"
	"geoguessme/internal/storage"
)

const routeTestMetricsToken = "route-test-metrics-token-0123456789ab"

func newTestMetrics() *middleware.Metrics {
	metrics := &middleware.Metrics{}
	metrics.Observe(http.StatusOK)
	metrics.Observe(http.StatusInternalServerError)
	metrics.SetCleanupBacklog(2)
	return metrics
}

func newLocalStore(t *testing.T) storage.ObjectStore {
	t.Helper()
	store, err := storage.NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("create local store: %v", err)
	}
	return store
}

func serveSystem(t *testing.T, cfg *config.Config, store storage.ObjectStore, method, target string, auth string) *httptest.ResponseRecorder {
	t.Helper()
	metrics := newTestMetrics()
	mux := http.NewServeMux()
	registerSystemRoutes(mux, cfg, metrics, store)

	request := httptest.NewRequest(method, target, nil)
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	return recorder
}

func TestRouteMetricsOpenInDevelopment(t *testing.T) {
	cfg := &config.Config{Environment: config.EnvDevelopment}
	recorder := serveSystem(t, cfg, newLocalStore(t), http.MethodGet, "/metrics", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("development /metrics status = %d, want 200", recorder.Code)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("development /metrics content-type = %q", got)
	}
	body := recorder.Body.String()
	for _, line := range []string{"geoguessme_http_requests_total 2", "geoguessme_http_errors_total 1", "geoguessme_storage_cleanup_backlog 2"} {
		if !strings.Contains(body, line) {
			t.Errorf("metrics body missing %q in %q", line, body)
		}
	}
	// An open endpoint must not advertise Bearer auth.
	if got := recorder.Header().Get("WWW-Authenticate"); got != "" {
		t.Errorf("development /metrics should not set WWW-Authenticate, got %q", got)
	}
}

func TestRouteMetricsOpenInTest(t *testing.T) {
	cfg := &config.Config{Environment: config.EnvTest}
	recorder := serveSystem(t, cfg, newLocalStore(t), http.MethodGet, "/metrics", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("test /metrics status = %d, want 200", recorder.Code)
	}
}

func TestRouteMetricsRequiresBearerInProduction(t *testing.T) {
	cfg := &config.Config{Environment: config.EnvProduction, MetricsToken: routeTestMetricsToken}

	// Missing token: 401 with protection headers.
	recorder := serveSystem(t, cfg, newLocalStore(t), http.MethodGet, "/metrics", "")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("production /metrics without token status = %d, want 401", recorder.Code)
	}
	if got := recorder.Header().Get("WWW-Authenticate"); got != "Bearer" {
		t.Fatalf("WWW-Authenticate = %q, want Bearer", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if !strings.Contains(recorder.Body.String(), "unauthorized") {
		t.Fatalf("rejection body = %q", recorder.Body.String())
	}

	// Wrong token (same length, different content): still 401.
	recorder = serveSystem(t, cfg, newLocalStore(t), http.MethodGet, "/metrics", "Bearer "+strings.Repeat("z", len(routeTestMetricsToken)))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("production /metrics with wrong token status = %d, want 401", recorder.Code)
	}
	if recorder.Header().Get("WWW-Authenticate") != "Bearer" || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("wrong-token rejection must keep protection headers")
	}

	// Correct token: metrics served.
	recorder = serveSystem(t, cfg, newLocalStore(t), http.MethodGet, "/metrics", "Bearer "+routeTestMetricsToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("production /metrics with token status = %d, want 200", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "geoguessme_http_requests_total") {
		t.Fatalf("metrics body = %q", recorder.Body.String())
	}
}

func TestRouteHealthLiveAlwaysOK(t *testing.T) {
	cfg := &config.Config{Environment: config.EnvDevelopment}
	recorder := serveSystem(t, cfg, newLocalStore(t), http.MethodGet, "/health/live", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("/health/live status = %d, want 200", recorder.Code)
	}
	if body := recorder.Body.String(); body != "ok\n" {
		t.Fatalf("/health/live body = %q, want %q", body, "ok\n")
	}
}

func TestRouteHealthReadyReportsAvailability(t *testing.T) {
	store := newLocalStore(t)
	cfg := &config.Config{Environment: config.EnvDevelopment}

	// No database connection registered: readiness fails.
	previousDB := database.DB
	database.DB = nil
	recorder := serveSystem(t, cfg, store, http.MethodGet, "/health/ready", "")
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("/health/ready without database status = %d, want 503", recorder.Code)
	}
	if body := recorder.Body.String(); body != "not ready\n" {
		t.Fatalf("/health/ready body = %q, want %q", body, "not ready\n")
	}

	// Healthy dependencies: readiness succeeds.
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("create mock pool: %v", err)
	}
	database.DB = mock
	t.Cleanup(func() {
		_ = mock.ExpectationsWereMet()
		mock.Close()
		database.DB = previousDB
	})
	mock.ExpectPing()
	recorder = serveSystem(t, cfg, store, http.MethodGet, "/health/ready", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("/health/ready with healthy deps status = %d, want 200", recorder.Code)
	}
	if body := recorder.Body.String(); body != "ready\n" {
		t.Fatalf("/health/ready body = %q, want %q", body, "ready\n")
	}
}

func TestRouteTestControlsGatedByEnvironment(t *testing.T) {
	store := newLocalStore(t)

	// Non-test environments must not register the control routes.
	cfg := &config.Config{Environment: config.EnvDevelopment}
	metrics := newTestMetrics()
	mux := http.NewServeMux()
	registerSystemRoutes(mux, cfg, metrics, store)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/test/rate-limit/reset", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("development test control status = %d, want 404", recorder.Code)
	}

	// Test environment registers the control route and clears the limiter.
	testCfg := &config.Config{Environment: config.EnvTest}
	testMux := http.NewServeMux()
	registerSystemRoutes(testMux, testCfg, newTestMetrics(), store)
	recorder = httptest.NewRecorder()
	testMux.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/test/rate-limit/reset", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("test control status = %d, want 200", recorder.Code)
	}
}
