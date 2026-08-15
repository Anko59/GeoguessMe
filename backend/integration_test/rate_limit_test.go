package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// rateLimitLoginBody returns a login payload that will always fail auth (wrong
// password) so tests can drive repeated requests without creating sessions.
func rateLimitLoginBody(identity string) map[string]string {
	return map[string]string{"username": identity, "password": "wrong"}
}

// postControl sends a POST to one of the test-only rate-limit control
// endpoints. It uses an independent context with a timeout so it also works
// from t.Cleanup, where t.Context() is already canceled.
func postControl(t *testing.T, path string, body any) {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+path, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "control endpoint %s returned %d", path, resp.StatusCode)
}

// resetRateLimiter calls the test-only endpoint to clear all rate-limit state.
func resetRateLimiter(t *testing.T) {
	t.Helper()
	postControl(t, "/api/v1/test/rate-limit/reset", nil)
}

// advanceClock calls the test-only endpoint to move the rate-limiter clock
// forward by the given number of seconds.
func advanceClock(t *testing.T, seconds int) {
	t.Helper()
	postControl(t, "/api/v1/test/rate-limit/clock/advance", map[string]int{"seconds": seconds})
}

// assertRateLimited asserts the F-04 429 contract: the existing error envelope
// plus a Retry-After header that lies within the limiting bucket's window.
func assertRateLimited(t *testing.T, resp jsonResponse, data []byte, maxRetryAfter int) {
	t.Helper()
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "expected 429, got %d", resp.StatusCode)

	ra := resp.Header.Get("Retry-After")
	require.NotEmpty(t, ra, "Retry-After header is missing on 429 response")
	seconds, err := strconv.Atoi(ra)
	require.NoError(t, err, "Retry-After must be an integer, got %q", ra)
	require.GreaterOrEqual(t, seconds, 1, "Retry-After must be at least 1 second")
	require.LessOrEqual(t, seconds, maxRetryAfter, "Retry-After must not exceed the bucket window")

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

// TestLoginIdentityLimitBlocksEleventh proves the login policy's identity
// bucket (10/min) end to end: ten failed logins for one identity pass and the
// eleventh is rejected with the error envelope and a bounded Retry-After.
func TestLoginIdentityLimitBlocksEleventh(t *testing.T) {
	resetRateLimiter(t)
	t.Cleanup(func() { resetRateLimiter(t) })

	identity := unique("rl-login")
	body := rateLimitLoginBody(identity)

	for i := range 10 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
		require.Equalf(t, http.StatusUnauthorized, resp.StatusCode,
			"request %d must return 401 (wrong password), got %d", i+1, resp.StatusCode)
	}

	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
	assertRateLimited(t, resp, data, 60)
}

// TestLoginIdentityIsolationBetweenIdentities proves identity buckets are
// independent: after Alice exhausts her quota, Bob under the same IP still has
// his full identity quota.
func TestLoginIdentityIsolationBetweenIdentities(t *testing.T) {
	resetRateLimiter(t)
	t.Cleanup(func() { resetRateLimiter(t) })

	alice := unique("rl-alice")
	bob := unique("rl-bob")

	for range 10 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", rateLimitLoginBody(alice), "", nil)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "alice below-limit must return 401")
	}
	resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", rateLimitLoginBody(alice), "", nil)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "alice must be rate-limited after her quota")

	for range 10 {
		resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/login", rateLimitLoginBody(bob), "", nil)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "bob below-limit must return 401")
	}
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/login", rateLimitLoginBody(bob), "", nil)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "bob must be independently rate-limited")
}

// TestLoginWindowResetAfterMinute proves the identity window resets: after
// advancing the test clock past the one-minute window the quota is restored.
func TestLoginWindowResetAfterMinute(t *testing.T) {
	resetRateLimiter(t)
	t.Cleanup(func() { resetRateLimiter(t) })

	identity := unique("rl-window")
	body := rateLimitLoginBody(identity)

	for range 10 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "below-limit must return 401")
	}
	resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode, "must be limited after exhausting quota")

	advanceClock(t, 61)

	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode,
		"request after window reset must return 401 (wrong password), got %d", resp.StatusCode)
}

// TestRateLimitResetClearsAllState proves the test-only reset wipes every
// bucket so the full quota is available again.
func TestRateLimitResetClearsAllState(t *testing.T) {
	resetRateLimiter(t)
	t.Cleanup(func() { resetRateLimiter(t) })

	identity := unique("rl-reset")
	body := rateLimitLoginBody(identity)

	for range 10 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "below-limit must return 401")
	}
	resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

	resetRateLimiter(t)

	for range 10 {
		resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "after reset must return 401")
	}
}

// TestRateLimitReturnsContentTypeJSON pins the 429 response's JSON content
// type alongside the envelope.
func TestRateLimitReturnsContentTypeJSON(t *testing.T) {
	resetRateLimiter(t)
	t.Cleanup(func() { resetRateLimiter(t) })

	identity := unique("rl-ctype")
	body := rateLimitLoginBody(identity)

	for range 10 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "below-limit must return 401")
	}
	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/login", body, "", nil)
	assertRateLimited(t, resp, data, 60)

	ct := resp.Header.Get("Content-Type")
	require.Contains(t, ct, "application/json", "429 response must have JSON Content-Type, got %q", ct)
}

// TestLoginGlobalCapHoldsUnderConcurrency proves the login global bucket
// (300/min, the mandated limit configured in the test stack) is exact under a
// concurrent burst: 400 distinct identities produce exactly 300 admitted and
// 100 rejected requests.
func TestLoginGlobalCapHoldsUnderConcurrency(t *testing.T) {
	resetRateLimiter(t)
	t.Cleanup(func() { resetRateLimiter(t) })

	const total = 400
	client := &http.Client{}
	results := make([]int, total)
	var wg sync.WaitGroup
	for i := range total {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := rateLimitLoginBody(fmt.Sprintf("rl-conc-%d", idx))
			raw, _ := json.Marshal(body)
			var code int
			for attempt := 0; attempt < 3; attempt++ {
				req, err := http.NewRequestWithContext(context.Background(),
					http.MethodPost, baseURL+"/api/v1/auth/login", bytes.NewReader(raw))
				if err != nil {
					code = -1
					break
				}
				req.Header.Set("Content-Type", "application/json")
				resp, doErr := client.Do(req)
				if doErr != nil {
					code = -1
					// Transient connection errors never consumed quota; retry.
					continue
				}
				code = resp.StatusCode
				_ = resp.Body.Close()
				break
			}
			results[idx] = code
		}(i)
	}
	wg.Wait()

	ok, limited := 0, 0
	for _, code := range results {
		switch code {
		case http.StatusUnauthorized:
			ok++
		case http.StatusTooManyRequests:
			limited++
		}
	}
	require.Equal(t, total, ok+limited, "every request must receive a definitive answer")
	require.Equal(t, 300, ok, "global 300/min cap must hold exactly under contention")
	require.Equal(t, total-300, limited, "the excess concurrent requests must be rejected")
}

// TestSignupIdentityLimitThreePerHour proves the signup identity bucket
// (3/hour): the fourth signup attempt with the same identity is rejected
// regardless of the first attempt's success.
func TestSignupIdentityLimitThreePerHour(t *testing.T) {
	resetRateLimiter(t)
	t.Cleanup(func() { resetRateLimiter(t) })

	username := unique("rl-signup")
	email := unique("rl") + "@test.local"
	body := map[string]string{"username": username, "email": email, "password": "StrongPassword123"}

	for range 3 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/signup", body, "", nil)
		require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode,
			"signup below the identity limit must not be rate-limited")
	}

	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/signup", body, "", nil)
	assertRateLimited(t, resp, data, 3600)
}

// TestRecoveryEmailLimitThreePerHour proves the recovery-email endpoint
// (password/forgot) enforces the email policy's 3/hour identity bucket.
func TestRecoveryEmailLimitThreePerHour(t *testing.T) {
	resetRateLimiter(t)
	t.Cleanup(func() { resetRateLimiter(t) })

	email := unique("rl-recover") + "@test.local"
	body := map[string]string{"email": email}

	for range 3 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/password/forgot", body, "", nil)
		require.Equal(t, http.StatusAccepted, resp.StatusCode, "forgot returns the uniform 202 below the limit")
	}

	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/password/forgot", body, "", nil)
	assertRateLimited(t, resp, data, 3600)
}

// TestVerificationEmailLimitThreePerHour proves the verification-email endpoint
// (verify/request) enforces the email policy's 3/hour bucket keyed by the
// authenticated user, whose requests carry no identity field in the body.
func TestVerificationEmailLimitThreePerHour(t *testing.T) {
	resetRateLimiter(t)
	t.Cleanup(func() { resetRateLimiter(t) })

	session := signup(t, unique("rl-verif"), unique("rl")+"@test.local", "StrongPassword123")

	for range 3 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/verify/request", map[string]string{}, session.access, nil)
		require.Equal(t, http.StatusAccepted, resp.StatusCode, "verify/request returns 202 below the limit")
	}

	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/verify/request", map[string]string{}, session.access, nil)
	assertRateLimited(t, resp, data, 3600)
}

// TestPasswordResetLimitTenPerIP proves the password-reset route enforces the
// reset policy's 10/hour trusted-IP bucket (the suite runs through one
// gateway address, so every attempt shares the IP key).
func TestPasswordResetLimitTenPerIP(t *testing.T) {
	resetRateLimiter(t)
	t.Cleanup(func() { resetRateLimiter(t) })

	body := map[string]string{"token": "garbage-token", "password": "BrandNewPassword123"}
	for range 10 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/auth/password/reset", body, "", nil)
		require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode,
			"reset below the limit must be rejected by the handler, not the limiter")
	}

	resp, data := doJSON(t, http.MethodPost, "/api/v1/auth/password/reset", body, "", nil)
	assertRateLimited(t, resp, data, 3600)
}

// TestPushSubscriptionLimitTenPerUser proves the push policy's user bucket
// (10/hour) applies to both subscribe and unsubscribe, and that two users are
// limited independently. Push delivery itself is disabled in the test stack,
// so in-limit requests answer with 503; the limiter runs before the handler.
func TestPushSubscriptionLimitTenPerUser(t *testing.T) {
	resetRateLimiter(t)
	t.Cleanup(func() { resetRateLimiter(t) })

	userA := signup(t, unique("rl-push-a"), unique("rl")+"@test.local", "StrongPassword123")
	userB := signup(t, unique("rl-push-b"), unique("rl")+"@test.local", "StrongPassword123")
	body := map[string]string{"endpoint": "https://push.example.com/sub", "p256dh": "BPnq2S63LQjVYp6B2M7wJhYQ8wTd0rUoIs4hW9s5cN0YxMm", "auth": "0x8fW2vQ3kL9mN4sT7"}

	for range 10 {
		resp, _ := doJSON(t, http.MethodPost, "/api/v1/push/subscribe", body, userA.access, nil)
		require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode,
			"subscribe below the user limit must not be rate-limited")
	}
	resp, data := doJSON(t, http.MethodPost, "/api/v1/push/subscribe", body, userA.access, nil)
	assertRateLimited(t, resp, data, 3600)

	// The other user's quota is untouched.
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/push/subscribe", body, userB.access, nil)
	require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode,
		"a second user must have an independent push quota")

	// Unsubscribe shares the same policy and user bucket, so user B has one
	// request left in the 10/hour bucket: nine unsubscribes pass and the tenth
	// is rejected.
	for range 9 {
		resp, _ = doJSON(t, http.MethodPost, "/api/v1/push/unsubscribe", body, userB.access, nil)
		require.NotEqual(t, http.StatusTooManyRequests, resp.StatusCode,
			"unsubscribe below the user limit must not be rate-limited")
	}
	resp, data = doJSON(t, http.MethodPost, "/api/v1/push/unsubscribe", body, userB.access, nil)
	assertRateLimited(t, resp, data, 3600)
}
