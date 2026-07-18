package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsAndRequestID(t *testing.T) {
	metrics := &Metrics{}
	metrics.Observe(http.StatusOK)
	metrics.Observe(http.StatusInternalServerError)
	metrics.SetCleanupBacklog(3)
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	recorder := httptest.NewRecorder()
	metrics.Handler(recorder, request)
	body := recorder.Body.String()
	for _, line := range []string{"geoguessme_http_requests_total 2", "geoguessme_http_errors_total 1", "geoguessme_storage_cleanup_backlog 3"} {
		if !strings.Contains(body, line) {
			t.Errorf("metrics missing %q in %q", line, body)
		}
	}

	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Context().Value(requestIDKey{}) == nil {
			t.Error("request ID was not placed in context")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Request-ID", "known-id")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Header().Get("X-Request-ID") != "known-id" {
		t.Fatalf("request ID = %q", recorder.Header().Get("X-Request-ID"))
	}
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	recorder = httptest.NewRecorder()
	RequestID(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(recorder, request)
	if recorder.Header().Get("X-Request-ID") == "" {
		t.Fatal("generated request ID is empty")
	}
}

func TestMetricsAuth(t *testing.T) {
	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("metrics"))
	})
	token := strings.Repeat("s", minMetricsTokenBytesForTest)

	assertRejection := func(t *testing.T, name string, request *http.Request) {
		t.Helper()
		handler := MetricsAuth(token, okHandler)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s: expected 401, got %d", name, recorder.Code)
		}
		if got := recorder.Header().Get("WWW-Authenticate"); got != "Bearer" {
			t.Fatalf("%s: WWW-Authenticate = %q, want Bearer", name, got)
		}
		if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("%s: Cache-Control = %q, want no-store", name, got)
		}
		if !strings.Contains(recorder.Body.String(), "unauthorized") {
			t.Fatalf("%s: body = %q", name, recorder.Body.String())
		}
	}

	// Without token, request is rejected with protection headers.
	assertRejection(t, "no token", httptest.NewRequest(http.MethodGet, "/metrics", nil))

	// Wrong token (same length, differs in content) is rejected in constant time.
	wrong := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	wrong.Header.Set("Authorization", "Bearer "+strings.Repeat("x", len(token)))
	assertRejection(t, "wrong token", wrong)

	// Wrong-length token is rejected.
	short := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	short.Header.Set("Authorization", "Bearer short")
	assertRejection(t, "short token", short)

	// Missing Bearer prefix.
	bare := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	bare.Header.Set("Authorization", token)
	assertRejection(t, "bare token", bare)

	// Case-insensitive scheme, correct token succeeds and reaches the handler.
	handler := MetricsAuth(token, okHandler)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.Header.Set("Authorization", "bearer "+token)
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "metrics" {
		t.Fatalf("expected 200 with correct token, got %d %q", recorder.Code, recorder.Body.String())
	}
}

// minMetricsTokenBytesForTest is independent of the config constant so this
// package does not import config just for a length.
const minMetricsTokenBytesForTest = 32

func TestRecoverAndRequestLog(t *testing.T) {
	recorder := httptest.NewRecorder()
	Recover(slog.Default(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "internal_error") {
		t.Fatalf("panic response = %d %q", recorder.Code, recorder.Body.String())
	}

	metrics := &Metrics{}
	recorder = httptest.NewRecorder()
	RequestLog(nil, metrics, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK || metrics.requests.Load() != 1 {
		t.Fatalf("logged response = %d, requests = %d", recorder.Code, metrics.requests.Load())
	}

	writer := &statusWriter{ResponseWriter: httptest.NewRecorder()}
	if _, err := writer.Write([]byte("body")); err != nil || writer.status != http.StatusOK {
		t.Fatalf("implicit write status = %d, err = %v", writer.status, err)
	}
	writer.WriteHeader(http.StatusAccepted)
	if writer.Unwrap() == nil {
		t.Fatal("unwrap returned nil")
	}
	writer.Flush()
	if _, _, err := writer.Hijack(); err == nil {
		t.Fatal("unsupported hijack unexpectedly succeeded")
	}
}

func TestCORSSecurityAndMiddlewareError(t *testing.T) {
	base := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	allowed := CORS([]string{"https://app.test"})(base)
	request := httptest.NewRequest(http.MethodOptions, "/", nil)
	request.Header.Set("Origin", "https://app.test")
	recorder := httptest.NewRecorder()
	allowed.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Header().Get("Access-Control-Allow-Origin") != "https://app.test" {
		t.Fatalf("allowed CORS response = %d %q", recorder.Code, recorder.Header().Get("Access-Control-Allow-Origin"))
	}

	request = httptest.NewRequest(http.MethodOptions, "/", nil)
	request.Header.Set("Origin", "https://evil.test")
	recorder = httptest.NewRecorder()
	allowed.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("disallowed CORS status = %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	SecurityHeaders(base).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Header().Get("X-Frame-Options") != "DENY" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers missing")
	}

	recorder = httptest.NewRecorder()
	writeMiddlewareError(recorder, http.StatusBadRequest, "bad", "message")
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), `"code":"bad"`) {
		t.Fatal("middleware error was not encoded")
	}
}
