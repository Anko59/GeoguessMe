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

	now := time.Now()
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
			lastReset: time.Now(),
		}
		rl.requests[key] = client
	}
	rl.mu.Unlock()

	client.mu.Lock()
	defer client.mu.Unlock()

	now := time.Now()
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
