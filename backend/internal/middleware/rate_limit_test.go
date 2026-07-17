package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRateLimit(t *testing.T) {
	// Create a rate limiter with 2 requests per second
	limit := 2
	window := time.Second
	middleware := RateLimit(limit, window)

	// Create a dummy handler
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test Request 1 (Allowed)
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.RemoteAddr = "1.2.3.4:1234"
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	assert.Equal(t, http.StatusOK, rr1.Code)

	// Test Request 2 (Allowed)
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "1.2.3.4:5678" // Same IP, different port
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	assert.Equal(t, http.StatusOK, rr2.Code)

	// Test Request 3 (Blocked)
	req3 := httptest.NewRequest("GET", "/", nil)
	req3.RemoteAddr = "1.2.3.4:9999"
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	assert.Equal(t, http.StatusTooManyRequests, rr3.Code)
	assert.Equal(t, "1", rr3.Header().Get("Retry-After"))

	// Test Request from different IP (Allowed)
	req4 := httptest.NewRequest("GET", "/", nil)
	req4.RemoteAddr = "5.6.7.8:1234"
	rr4 := httptest.NewRecorder()
	handler.ServeHTTP(rr4, req4)
	assert.Equal(t, http.StatusOK, rr4.Code)
}

func TestRateLimitHeaders(t *testing.T) {
	limit := 1
	window := time.Second
	middleware := RateLimit(limit, window)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Request 1 (Allowed)
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.Header.Set("X-Forwarded-For", "10.0.0.1")
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)
	assert.Equal(t, http.StatusOK, rr1.Code)

	// Request 2 (Blocked - same X-Forwarded-For)
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.Header.Set("X-Forwarded-For", "10.0.0.1")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rr2.Code)
}
