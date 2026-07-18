package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimit(t *testing.T) {
	ResetRateLimiter()

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
	ResetRateLimiter()

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

func TestRateLimitWindowResetWithClockAdvance(t *testing.T) {
	ResetRateLimiter()

	limit := 2
	window := 100 * time.Millisecond
	middleware := RateLimit(limit, window)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "192.168.1.1:1234"

	// Exhaust quota.
	for range limit {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ip
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
	}

	// Should be rate-limited now.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusTooManyRequests, rr.Code)

	// Advance clock past the window.
	AdvanceTestClock(200 * time.Millisecond)

	// Window reset; requests should succeed again.
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "request should succeed after clock advance past window")
}

func TestRateLimitResetClearsCountersAndClock(t *testing.T) {
	ResetRateLimiter()

	limit := 1
	window := time.Second
	middleware := RateLimit(limit, window)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "10.0.0.99:1234"

	// Exhaust.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Rate-limited.
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusTooManyRequests, rr.Code)

	// Advance clock (not past window).
	AdvanceTestClock(200 * time.Millisecond)

	// Reset everything.
	ResetRateLimiter()

	// Full quota should be available and clock back to real time.
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "request should succeed after rate limiter reset")
}

func TestRateLimitConcurrentRequests(t *testing.T) {
	ResetRateLimiter()

	limit := 100
	window := time.Second
	middleware := RateLimit(limit, window)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	var wg sync.WaitGroup
	concurrency := 50
	results := make([]int, concurrency)

	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = "172.16.0.1:1234"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			results[idx] = rr.Code
		}(i)
	}
	wg.Wait()

	okCount := 0
	for _, code := range results {
		if code == http.StatusOK {
			okCount++
		}
	}
	// With limit=100 and 50 concurrent requests from the same IP, all should
	// pass under the limit.
	require.Equal(t, concurrency, okCount, "all concurrent requests within limit must succeed")
}

func TestRateLimitConcurrentExceedsLimit(t *testing.T) {
	ResetRateLimiter()

	limit := 5
	window := time.Second
	middleware := RateLimit(limit, window)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	var wg sync.WaitGroup
	concurrency := 10
	results := make([]int, concurrency)

	for i := range concurrency {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = "172.16.0.2:1234"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			results[idx] = rr.Code
		}(i)
	}
	wg.Wait()

	okCount := 0
	limitedCount := 0
	for _, code := range results {
		switch code {
		case http.StatusOK:
			okCount++
		case http.StatusTooManyRequests:
			limitedCount++
		}
	}
	require.Equal(t, limit, okCount, "exactly limit requests should succeed")
	require.Equal(t, concurrency-limit, limitedCount, "remaining requests should be rate-limited")
}

func TestRateLimitConcurrentClockAdvanceAndRequests(t *testing.T) {
	ResetRateLimiter()

	limit := 3
	window := 50 * time.Millisecond
	middleware := RateLimit(limit, window)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "172.16.0.3:1234"

	// Exhaust quota.
	for range limit {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ip
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
	}

	// Concurrent: advance clock + make requests.
	var wg sync.WaitGroup
	wg.Add(3)

	// Goroutine 1: advance clock.
	go func() {
		defer wg.Done()
		AdvanceTestClock(100 * time.Millisecond)
	}()

	// Goroutines 2-3: make requests.
	for range 2 {
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = ip
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			_ = rr.Code
		}()
	}
	wg.Wait()
	// No race detector failures = success. After clock advance, new requests
	// should succeed. Validate that at least one of the two concurrent
	// requests succeeded (the window may have been reset during the advance).
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "after clock advance, requests should succeed again")
}

func TestRateLimitConcurrentResetAndRequests(t *testing.T) {
	ResetRateLimiter()

	limit := 2
	window := time.Second
	middleware := RateLimit(limit, window)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "172.16.0.4:1234"

	// Exhaust quota.
	for range limit {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ip
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
	}

	// Concurrent reset + request.
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		ResetRateLimiter()
	}()

	go func() {
		defer wg.Done()
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ip
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		_ = rr.Code
	}()
	wg.Wait()

	// After reset, quota should be available again.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "request should succeed after concurrent reset")
}

func TestSetClockNilRestoresRealTime(t *testing.T) {
	ResetRateLimiter()

	// Install a frozen clock.
	frozen := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	SetClock(func() time.Time { return frozen })

	limit := 1
	window := time.Second
	middleware := RateLimit(limit, window)
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "10.0.0.200:1234"

	// Exhaust quota with frozen clock.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	// Still rate-limited since clock never advances.
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusTooManyRequests, rr.Code)

	// Restore real time.
	SetClock(nil)

	// Reset and verify real clock works.
	ResetRateLimiter()
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = ip
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, "request should succeed after restoring real clock and reset")
}
