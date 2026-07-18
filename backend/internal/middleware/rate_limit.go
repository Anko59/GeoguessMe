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
	mu       sync.Mutex // protects all fields below
	clock    func() time.Time
	offset   time.Duration
	start    time.Time
}

type clientRate struct {
	count     int
	lastReset time.Time
}

var limiter = &rateLimiter{
	requests: make(map[string]*clientRate),
	clock:    time.Now,
}

// cleanupInterval is how often stale entries are pruned.
const cleanupInterval = 10 * time.Minute

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

	now := rl.clock()
	for key, client := range rl.requests {
		if now.Sub(client.lastReset) > 5*time.Minute {
			delete(rl.requests, key)
		}
	}
}

func (rl *rateLimiter) allow(key string, limit int, window time.Duration) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.clock()
	client, exists := rl.requests[key]
	if !exists {
		client = &clientRate{
			count:     0,
			lastReset: now,
		}
		rl.requests[key] = client
	}

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

// RateLimit creates a rate limiting middleware that uses the client IP
// address (including X-Forwarded-For without proxy validation for legacy
// callers) as the rate-limit key.
func RateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	return rateLimit(limit, window, nil, true)
}

// RateLimitWithTrustedProxies only accepts forwarded client IP headers when
// the immediate peer is in a configured proxy network.
func RateLimitWithTrustedProxies(limit int, window time.Duration, trustedCIDRs []string) func(http.Handler) http.Handler {
	return rateLimit(limit, window, trustedCIDRs, false)
}

// RateLimitByIdentity rate-limits by client IP combined with the identity
// field (username or email) extracted from the JSON request body (up to
// 64 KiB). The body is replaced so downstream handlers can still read it.
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
// the default time.Now. The caller must not hold the limiter lock.
func SetClock(fn func() time.Time) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if fn == nil {
		limiter.clock = time.Now
		limiter.offset = 0
		return
	}
	limiter.clock = fn
}

// ResetRateLimiter clears every tracked client and restores the real-time
// clock so tests can start from a clean slate. Production code must not
// call it.
func ResetRateLimiter() {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	limiter.requests = make(map[string]*clientRate)
	limiter.clock = time.Now
	limiter.offset = 0
}

// AdvanceTestClock moves the rate-limiter clock forward by d. When the test
// clock has never been set the call anchors the simulated clock at the
// current wall-clock time and then advances it. Production code must not
// call this.
func AdvanceTestClock(d time.Duration) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	// On the first call (or first after reset), anchor to wall-clock time.
	if limiter.offset == 0 {
		limiter.start = time.Now()
	}
	limiter.offset += d
	limiter.clock = func() time.Time { return limiter.start.Add(limiter.offset) }
}

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
