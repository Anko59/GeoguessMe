package middleware

import (
	"net"
	"net/http"
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
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use IP address as key
			key := r.RemoteAddr
			host, _, err := net.SplitHostPort(key)
			if err == nil {
				key = host
			}

			// Try to get a more accurate IP from headers
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				key = forwarded
			} else if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
				key = realIP
			}

			if !limiter.allow(key, limit, window) {
				w.Header().Set("Retry-After", window.String())
				http.Error(w, "Too many requests", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
