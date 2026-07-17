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
