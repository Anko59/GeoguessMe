package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// rateLimitLoginBody returns a login payload that will always fail auth (wrong
// password) so tests can drive repeated requests without creating sessions.
func rateLimitLoginBody(identity string) map[string]string {
	return map[string]string{"username": identity, "password": "wrong"}
}

// resetRateLimiter calls the test-only endpoint to clear all rate-limit state.
func resetRateLimiter(t *testing.T) {
	t.Helper()
	resp, _ := doJSON(t, http.MethodPost, "/api/v1/test/rate-limit/reset", nil, "", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "reset endpoint returned %d", resp.StatusCode)
}

// advanceClock calls the test-only endpoint to move the rate-limiter clock
// forward by the given number of seconds.
func advanceClock(t *testing.T, seconds int) {
	t.Helper()
	resp, _ := doJSON(t, http.MethodPost, "/api/v1/test/rate-limit/clock/advance",
		map[string]int{"seconds": seconds}, "", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "clock advance endpoint returned %d", resp.StatusCode)
}

func TestRateLimitRequestsSucceedBelowLimit(t *testing.T) {
	resetRateLimiter(t)
	identity := unique("ratelimit")
	body := rateLimitLoginBody(identity)

	// Three requests at the configured limit of 3 must not be rate-limited.
	for i := range 3 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
		require.NotEqualf(t, http.StatusTooManyRequests, resp.StatusCode,
			"request %d should not be rate-limited", i+1)
	}
}

func TestRateLimitExceededReturns429(t *testing.T) {
	resetRateLimiter(t)
	identity := unique("ratelimit")
	body := rateLimitLoginBody(identity)

	// Exhaust the quota.
	for range 3 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
		require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode)
	}

	// Next request must be rejected.
	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode,
		"expected 429 after exhausting quota, got %d", resp.StatusCode)

	// Assert Retry-After header.
	ra := resp.Header.Get("Retry-After")
	require.NotEmpty(t, ra, "Retry-After header is missing on 429 response")
	require.Equal(t, "10", ra, "Retry-After should match the configured window")

	// Assert error body.
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(data, &envelope))
	require.Equal(t, "rate_limited", envelope.Error.Code)
	require.Equal(t, "Too many requests", envelope.Error.Message)
}

func TestRateLimitIsolationBetweenIdentities(t *testing.T) {
	resetRateLimiter(t)

	alice := unique("alice")
	bob := unique("bob")
	aliceBody := rateLimitLoginBody(alice)
	bobBody := rateLimitLoginBody(bob)

	// Exhaust Alice's quota.
	for range 3 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", aliceBody, "", nil)
		require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode)
	}

	// Alice is now rate-limited.
	resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", aliceBody, "", nil)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode,
		"alice should be rate-limited after exhausting quota")

	// Bob still has his full quota.
	for range 3 {
		resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/login", bobBody, "", nil)
		require.NotEqualf(t, http.StatusTooManyRequests, resp.StatusCode,
			"bob should not be rate-limited by alice's exhaustion")
	}

	// Bob's 4th request is rate-limited independently.
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/login", bobBody, "", nil)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode,
		"bob should be independently rate-limited after his own quota")
}

func TestRateLimitWindowReset(t *testing.T) {
	resetRateLimiter(t)
	identity := unique("ratelimit")
	body := rateLimitLoginBody(identity)

	// Exhaust the quota.
	for range 3 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
		require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode)
	}

	// Confirm limit is hit.
	resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

	// Advance the test clock past the 10-second window.
	advanceClock(t, 11)

	// Requests should be allowed again after the window resets.
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
	require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode,
		"request should succeed after window reset via clock advance")
}

func TestRateLimitResetClearsAllState(t *testing.T) {
	resetRateLimiter(t)
	identity := unique("ratelimit")
	body := rateLimitLoginBody(identity)

	// Exhaust the quota.
	for range 3 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
		require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode)
	}

	// Confirm limit is hit.
	resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

	// Reset.
	resetRateLimiter(t)

	// Full quota available again.
	for range 3 {
		resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
		require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode)
	}
}

func TestRateLimitReturnsContentTypeJSON(t *testing.T) {
	resetRateLimiter(t)
	identity := unique("ratelimit")
	body := rateLimitLoginBody(identity)

	// Exhaust quota.
	for range 3 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
		require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode)
	}

	resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

	ct := resp.Header.Get("Content-Type")
	require.Contains(t, ct, "application/json",
		"429 response must have JSON Content-Type, got %q", ct)
}
