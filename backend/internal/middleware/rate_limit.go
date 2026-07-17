package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rateLimiter struct {
	requests map[string]*clientRate
	mu       sync.RWMutex
}

type clientRate struct {
	count     int
	lastReset time.Time
	mu        sync.Mutex
}

var (
	limiter = &rateLimiter{
		requests: make(map[string]*clientRate),
	}
	// Cleanup old entries every 10 minutes
	cleanupInterval = 10 * time.Minute
	// clock is the time source; injected by tests that need deterministic time.
	clock func() time.Time = time.Now
)

func init() {
	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for range ticker.C {
			limiter.cleanup()
		}
	}()
}

func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := clock()
	for key, client := range rl.requests {
		client.mu.Lock()
		if now.Sub(client.lastReset) > time.Minute*5 {
			delete(rl.requests, key)
		}
		client.mu.Unlock()
	}
}

func (rl *rateLimiter) allow(key string, limit int, window time.Duration) bool {
	rl.mu.Lock()
	client, exists := rl.requests[key]
	if !exists {
		client = &clientRate{
			count:     0,
			lastReset: clock(),
		}
		rl.requests[key] = client
	}
	rl.mu.Unlock()

	client.mu.Lock()
	defer client.mu.Unlock()

	now := clock()
	if now.Sub(client.lastReset) > window {
		client.count = 0
		client.lastReset = now
	}

	if client.count >= limit {
		return false
	}

	client.count++
	return true
}

// RateLimit creates a rate limiting middleware
// limit: maximum number of requests
// window: time window for the limit
func RateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	return rateLimit(limit, window, nil, true)
}

// RateLimitWithTrustedProxies only accepts forwarded client IP headers when
// the immediate peer is in a configured proxy network.
func RateLimitWithTrustedProxies(limit int, window time.Duration, trustedCIDRs []string) func(http.Handler) http.Handler {
	return rateLimit(limit, window, trustedCIDRs, false)
}

func RateLimitByIdentity(limit int, window time.Duration, trustedCIDRs []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body []byte
			var err error
			if r.Body != nil {
				body, err = io.ReadAll(io.LimitReader(r.Body, 64*1024))
			}
			if err == nil {
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
			identity := ""
			var fields map[string]string
			if json.Unmarshal(body, &fields) == nil {
				identity = strings.ToLower(strings.TrimSpace(fields["username"]))
				if identity == "" {
					identity = strings.ToLower(strings.TrimSpace(fields["email"]))
				}
			}
			key := clientKey(r, trustedCIDRs) + "|" + identity
			if !limiter.allow(key, limit, window) {
				writeRateLimited(w, window)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func rateLimit(limit int, window time.Duration, trustedCIDRs []string, legacyForwarded bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use IP address as key
			key := clientKey(r, trustedCIDRs)
			if legacyForwarded {
				key = legacyClientKey(r)
			}

			if !limiter.allow(key, limit, window) {
				writeRateLimited(w, window)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeRateLimited(w http.ResponseWriter, window time.Duration) {
	w.Header().Set("Retry-After", retryAfterSeconds(window))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"error":{"code":"rate_limited","message":"Too many requests"}}`))
}

// retryAfterSeconds returns an integer-second Retry-After value per RFC 9110.
func retryAfterSeconds(window time.Duration) string {
	seconds := int64(window.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return strconv.FormatInt(seconds, 10)
}

func clientKey(r *http.Request, trustedCIDRs []string) string {
	key := r.RemoteAddr
	host, _, err := net.SplitHostPort(key)
	if err == nil {
		key = host
	}
	if trustedPeer(r, trustedCIDRs) {
		if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
			key = forwarded
		} else if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
			key = realIP
		}
	}
	return key
}

func legacyClientKey(r *http.Request) string {
	key := r.RemoteAddr
	host, _, err := net.SplitHostPort(key)
	if err == nil {
		key = host
	}
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		key = forwarded
	} else if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		key = realIP
	}
	return key
}

// SetClock replaces the rate-limiter time source. A nil function restores
// the default time.Now. Callers must ensure that concurrent rate-limit
// evaluations are not in-flight while the clock is being changed.
func SetClock(fn func() time.Time) {
	if fn == nil {
		clock = time.Now
		return
	}
	clock = fn
}

// ResetRateLimiter clears every tracked client so tests can start from a
// clean slate. This is a test seam; production code must not call it.
func ResetRateLimiter() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	limiter.requests = make(map[string]*clientRate)
}

// AdvanceTestClock moves an internal test clock forward by d. When the test
// clock has never been set the call creates a clock starting at the current
// wall-clock time and then advances it. Production code must not call this.
func AdvanceTestClock(d time.Duration) {
	// Capture the current wall-clock time once on first call so the
	// simulated clock starts from a known reference point.
	startOnce.Do(func() { testStart = time.Now() })
	// Re-read the stored start under the lock for safety; the Do above
	// guarantees it is set.
	testMu.Lock()
	defer testMu.Unlock()
	testOffset += d
	clock = func() time.Time { return testStart.Add(testOffset) }
}

var (
	testStart  time.Time
	testOffset time.Duration
	testMu     sync.Mutex
	startOnce  sync.Once
)

func trustedPeer(r *http.Request, cidrs []string) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	for _, cidr := range cidrs {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
		if err == nil && prefix.Contains(peer) {
			return true
		}
	}
	return false
}
