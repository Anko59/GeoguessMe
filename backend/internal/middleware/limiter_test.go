package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testIdentityHandler builds a policy middleware that reads the identity from
// a test header and the trusted IP from the socket address, then returns a
// closure issuing a request and capturing the response.
func testIdentityHandler(t *testing.T, p Policy) func(identity, ip string) *httptest.ResponseRecorder {
	t.Helper()
	mw := newPolicyMiddleware(p, keyExtractors{
		trustedIP: func(r *http.Request) string { return clientKey(r, nil) },
		identity: func(r *http.Request) string {
			return strings.ToLower(strings.TrimSpace(r.Header.Get("X-Test-Identity")))
		},
	})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	return func(identity, ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = ip + ":1234"
		req.Header.Set("X-Test-Identity", identity)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}
}

// testUserHandler is like testIdentityHandler but keys the user bucket from a
// test header, mirroring the authenticated-user dimension.
func testUserHandler(t *testing.T, p Policy) func(user, ip string) *httptest.ResponseRecorder {
	t.Helper()
	mw := newPolicyMiddleware(p, keyExtractors{
		trustedIP: func(r *http.Request) string { return clientKey(r, nil) },
		user:      func(r *http.Request) string { return r.Header.Get("X-Test-User") },
	})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	return func(user, ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/push/subscribe", nil)
		req.RemoteAddr = ip + ":1234"
		req.Header.Set("X-Test-User", user)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}
}

func loginPolicy() Policy {
	return Policy{Name: "login", FailClosed: true, Buckets: []BucketSpec{
		{Type: BucketIdentity, Limit: 10, Window: time.Minute},
		{Type: BucketTrustedIP, Limit: 30, Window: time.Minute},
		{Type: BucketGlobal, Limit: 300, Window: time.Minute},
	}}
}

func signupPolicy() Policy {
	return Policy{Name: "signup", FailClosed: true, Buckets: []BucketSpec{
		{Type: BucketIdentity, Limit: 3, Window: time.Hour},
		{Type: BucketTrustedIP, Limit: 5, Window: time.Hour},
		{Type: BucketGlobal, Limit: 60, Window: time.Minute},
	}}
}

func TestRateLimitLoginPolicyAggregates(t *testing.T) {
	ResetRateLimiter()
	defer ResetRateLimiter()

	do := testIdentityHandler(t, loginPolicy())

	// Identity bucket: 10/min per identity.
	for i := 1; i <= 10; i++ {
		require.Equal(t, http.StatusOK, do("alice", "1.2.3.4").Code)
	}
	rr := do("alice", "1.2.3.4")
	require.Equal(t, http.StatusTooManyRequests, rr.Code)
	// The identity window is one minute, so Retry-After is 60 seconds.
	require.Equal(t, "60", rr.Header().Get("Retry-After"))

	// A different identity under the same IP is still allowed until the IP
	// bucket (30/min) or global bucket (300/min) exhausts.
	require.Equal(t, http.StatusOK, do("bob", "1.2.3.4").Code)

	// IP bucket: requests from 1.2.3.4 now total 11 (10 alice + 1 bob);
	// 19 more distinct identities bring the IP counter to its 30/min limit.
	for i := 1; i <= 19; i++ {
		require.Equal(t, http.StatusOK, do(fmt.Sprintf("c%d", i), "1.2.3.4").Code)
	}
	// The 31st request from the same IP is blocked.
	require.Equal(t, http.StatusTooManyRequests, do("c20", "1.2.3.4").Code)

	// Global bucket: 300/min across all identities and IPs. The counter is at
	// 30 after the IP section, so 270 more requests fill it exactly.
	for i := 1; i <= 270; i++ {
		require.Equal(t, http.StatusOK, do(fmt.Sprintf("g%d", i), fmt.Sprintf("9.%d.%d.%d", i/65536, (i/256)%256, i%256)).Code)
	}
	// 300 allowed; the 301st is blocked by the global cap.
	require.Equal(t, http.StatusTooManyRequests, do("g271", "8.8.8.8").Code)
}

func TestRateLimitSignupMixedWindows(t *testing.T) {
	ResetRateLimiter()
	defer ResetRateLimiter()

	do := testIdentityHandler(t, signupPolicy())

	// Identity bucket: 3/hour per identity, IP bucket: 5/hour.
	for i := 1; i <= 3; i++ {
		require.Equal(t, http.StatusOK, do("alice", fmt.Sprintf("10.0.0.%d", i)).Code)
	}
	require.Equal(t, http.StatusTooManyRequests, do("alice", "10.0.0.9").Code)

	// The IP window is independent: five distinct identities from one IP pass,
	// and the sixth from the same IP is blocked even though every identity
	// bucket is below its limit.
	for i := 1; i <= 5; i++ {
		require.Equal(t, http.StatusOK, do(fmt.Sprintf("b%d", i), "10.1.1.1").Code)
	}
	require.Equal(t, http.StatusTooManyRequests, do("b6", "10.1.1.1").Code)
}

func TestRateLimitUserBucket(t *testing.T) {
	ResetRateLimiter()
	defer ResetRateLimiter()

	p := Policy{Name: "push", FailClosed: false, Buckets: []BucketSpec{
		{Type: BucketUser, Limit: 10, Window: time.Hour},
		{Type: BucketTrustedIP, Limit: 20, Window: time.Hour},
	}}
	do := testUserHandler(t, p)

	for i := 1; i <= 10; i++ {
		require.Equal(t, http.StatusOK, do("u1", fmt.Sprintf("10.0.0.%d", i)).Code)
	}
	require.Equal(t, http.StatusTooManyRequests, do("u1", "10.0.0.11").Code)
	require.Equal(t, http.StatusOK, do("u2", "10.0.0.12").Code)
}

func TestRateLimitConcurrentGlobalCapHolds(t *testing.T) {
	ResetRateLimiter()
	defer ResetRateLimiter()

	do := testIdentityHandler(t, loginPolicy())

	const total = 400
	var wg sync.WaitGroup
	results := make([]int, total)
	for i := range total {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ip := fmt.Sprintf("10.%d.%d.%d", idx/65536, (idx/256)%256, idx%256)
			results[idx] = do(fmt.Sprintf("u%d", idx), ip).Code
		}(i)
	}
	wg.Wait()

	ok, limited := 0, 0
	for _, code := range results {
		switch code {
		case http.StatusOK:
			ok++
		case http.StatusTooManyRequests:
			limited++
		}
	}
	require.Equal(t, 300, ok, "global 300/min cap must hold exactly under contention")
	require.Equal(t, total-300, limited, "the excess concurrent requests must be rejected")
}

func TestRateLimitStoreTTLEviction(t *testing.T) {
	ResetRateLimiter()
	defer ResetRateLimiter()

	do := testIdentityHandler(t, signupPolicy())

	for i := 1; i <= 3; i++ {
		require.Equal(t, http.StatusOK, do("alice", "1.2.3.4").Code)
	}
	// identity + trustedIP + global counters.
	require.Equal(t, 3, StoreSize())
	require.Equal(t, http.StatusTooManyRequests, do("alice", "1.2.3.5").Code)

	// Advance past every window (1h and 1m) and run the periodic sweeper the
	// way the background goroutine does.
	AdvanceTestClock(2 * time.Hour)
	limiterStore.mu.Lock()
	limiterStore.sweepLocked(limiterStore.clock())
	limiterStore.mu.Unlock()
	require.Equal(t, 0, StoreSize())

	// Expired counters are reclaimed, so the quota is available again.
	require.Equal(t, http.StatusOK, do("alice", "1.2.3.6").Code)
}

func TestRateLimitFailClosedOnCapacity(t *testing.T) {
	ResetRateLimiter()
	defer ResetRateLimiter()

	do := testIdentityHandler(t, loginPolicy())

	// Seed one live key through the normal path.
	require.Equal(t, http.StatusOK, do("alice", "1.2.3.4").Code)

	// Fill the store to its exact capacity with fresh, unexpired counters.
	limiterStore.mu.Lock()
	now := limiterStore.clock()
	filled := 0
	for len(limiterStore.buckets) < maxRateLimitBuckets {
		limiterStore.buckets[fmt.Sprintf("seed:%d", filled)] = &counter{count: 0, resetAt: now.Add(time.Hour)}
		filled++
	}
	require.Equal(t, maxRateLimitBuckets, len(limiterStore.buckets))
	limiterStore.mu.Unlock()

	// Existing keys keep working.
	require.Equal(t, http.StatusOK, do("alice", "1.2.3.4").Code)

	// A brand-new key is rejected (fail-closed) because no capacity remains.
	rr := do("mallory", "9.9.9.9")
	require.Equal(t, http.StatusTooManyRequests, rr.Code)
	require.NotEmpty(t, rr.Header().Get("Retry-After"))

	// The rejection is observable through the counter surface.
	require.Greater(t, Rejections(), int64(0))
	require.Contains(t, RejectionsByPolicy(), "login")
}

func TestRateLimitRetryAfterMinimumAcrossBuckets(t *testing.T) {
	ResetRateLimiter()
	defer ResetRateLimiter()

	p := Policy{Name: "mincheck", FailClosed: true, Buckets: []BucketSpec{
		{Type: BucketIdentity, Limit: 10, Window: time.Hour},
		{Type: BucketTrustedIP, Limit: 30, Window: time.Minute},
	}}
	do := testIdentityHandler(t, p)

	// Exhaust the identity bucket for alice.
	for i := 1; i <= 10; i++ {
		require.Equal(t, http.StatusOK, do("alice", "1.2.3.4").Code)
	}
	// Exhaust the IP bucket with distinct identities.
	for i := 1; i <= 20; i++ {
		require.Equal(t, http.StatusOK, do(fmt.Sprintf("b%d", i), "1.2.3.4").Code)
	}
	// Both are now exhausted for alice; the shorter IP window (60s) governs.
	rr := do("alice", "1.2.3.4")
	require.Equal(t, http.StatusTooManyRequests, rr.Code)
	require.Equal(t, "60", rr.Header().Get("Retry-After"))
}

func bucketLimit(p Policy, t BucketType) int {
	for _, b := range p.Buckets {
		if b.Type == t {
			return b.Limit
		}
	}
	return -1
}

func bucketWindow(p Policy, t BucketType) time.Duration {
	for _, b := range p.Buckets {
		if b.Type == t {
			return b.Window
		}
	}
	return 0
}

func TestDefaultPoliciesMatchMandatedLimits(t *testing.T) {
	policies := make(map[string]Policy, len(DefaultPolicies()))
	for _, p := range DefaultPolicies() {
		policies[p.Name] = p
	}

	login := policies["login"]
	require.Equal(t, 10, bucketLimit(login, BucketIdentity))
	require.Equal(t, time.Minute, bucketWindow(login, BucketIdentity))
	require.Equal(t, 30, bucketLimit(login, BucketTrustedIP))
	require.Equal(t, 300, bucketLimit(login, BucketGlobal))
	require.True(t, login.FailClosed)

	signup := policies["signup"]
	require.Equal(t, 3, bucketLimit(signup, BucketIdentity))
	require.Equal(t, time.Hour, bucketWindow(signup, BucketIdentity))
	require.Equal(t, 5, bucketLimit(signup, BucketTrustedIP))
	require.Equal(t, 60, bucketLimit(signup, BucketGlobal))
	require.True(t, signup.FailClosed)

	email := policies["email"]
	require.Equal(t, 3, bucketLimit(email, BucketIdentity))
	require.Equal(t, time.Hour, bucketWindow(email, BucketIdentity))
	require.Equal(t, 5, bucketLimit(email, BucketTrustedIP))
	require.Equal(t, 30, bucketLimit(email, BucketGlobal))
	require.True(t, email.FailClosed)

	reset := policies["reset"]
	require.Equal(t, 10, bucketLimit(reset, BucketTrustedIP))
	require.Equal(t, time.Hour, bucketWindow(reset, BucketTrustedIP))
	require.True(t, reset.FailClosed)

	push := policies["push"]
	require.Equal(t, 10, bucketLimit(push, BucketUser))
	require.Equal(t, time.Hour, bucketWindow(push, BucketUser))
	require.Equal(t, 20, bucketLimit(push, BucketTrustedIP))
	require.False(t, push.FailClosed)

	def := policies["default"]
	require.Equal(t, 10, bucketLimit(def, BucketIdentity))
	require.Equal(t, 60, bucketLimit(def, BucketTrustedIP))
	require.False(t, def.FailClosed)
}

func TestPolicyMiddlewareBodyIdentityAndUserBuckets(t *testing.T) {
	ResetRateLimiter()
	defer ResetRateLimiter()

	p := Policy{Name: "pm", FailClosed: false, Buckets: []BucketSpec{
		{Type: BucketIdentity, Limit: 2, Window: time.Minute},
		{Type: BucketUser, Limit: 1, Window: time.Minute},
	}}
	mw := PolicyMiddleware(p, PolicyOptions{
		User: func(r *http.Request) string { return r.Header.Get("X-Test-User") },
	})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	do := func(body, user string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/push/subscribe", strings.NewReader(body))
		req.RemoteAddr = "192.0.2.5:1234"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User", user)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	// The identity bucket counts by body identity: the first two requests for
	// the same email pass and the third is blocked even with distinct users.
	require.Equal(t, http.StatusOK, do(`{"email":"a@example.com"}`, "u1"))
	require.Equal(t, http.StatusOK, do(`{"email":"a@example.com"}`, "u2"))
	require.Equal(t, http.StatusTooManyRequests, do(`{"email":"a@example.com"}`, "u3"))

	// The user bucket is independent of identity: a fresh identity is still
	// blocked once the user quota is exhausted.
	require.Equal(t, http.StatusTooManyRequests, do(`{"email":"b@example.com"}`, "u1"))
}

func TestPolicyMiddlewareIdentityOverride(t *testing.T) {
	ResetRateLimiter()
	defer ResetRateLimiter()

	p := Policy{Name: "pm2", FailClosed: false, Buckets: []BucketSpec{
		{Type: BucketIdentity, Limit: 1, Window: time.Minute},
	}}
	mw := PolicyMiddleware(p, PolicyOptions{
		Identity: func(r *http.Request) string { return r.Header.Get("X-Identity-Override") },
	})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	do := func(override string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify/request", nil)
		req.RemoteAddr = "192.0.2.6:1234"
		req.Header.Set("X-Identity-Override", override)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	require.Equal(t, http.StatusOK, do("user-1"))
	require.Equal(t, http.StatusTooManyRequests, do("user-1"))
	require.Equal(t, http.StatusOK, do("user-2"))
}

func TestExtractIdentityExported(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"  Alice  ","password":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	require.Equal(t, "alice", ExtractIdentity(req))

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password/forgot", strings.NewReader(`{"email":" Bob@Example.COM "}`))
	req2.Header.Set("Content-Type", "application/json")
	require.Equal(t, "bob@example.com", ExtractIdentity(req2))

	// Non-JSON bodies carry no identity; the wrapper returns the empty string.
	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("username=alice"))
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	require.Equal(t, "", ExtractIdentity(req3))

	req4 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", nil)
	require.Equal(t, "", ExtractIdentity(req4))
}
