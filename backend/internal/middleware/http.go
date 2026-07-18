package middleware

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

type Metrics struct {
	requests       atomic.Uint64
	errors         atomic.Uint64
	cleanupBacklog atomic.Int64
}

func (m *Metrics) Handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("geoguessme_http_requests_total " + formatUint(m.requests.Load()) + "\n" + "geoguessme_http_errors_total " + formatUint(m.errors.Load()) + "\n" + "geoguessme_storage_cleanup_backlog " + formatUint(uint64(m.cleanupBacklog.Load())) + "\n"))
}

func (m *Metrics) Observe(status int) {
	m.requests.Add(1)
	if status >= 500 {
		m.errors.Add(1)
	}
}

// MetricsAuth returns a handler that requires a Bearer token matching the
// configured token before delegating to next. The presented token is compared
// to the configured value in constant time so a caller cannot learn the token
// through timing side channels. Rejected responses carry WWW-Authenticate and
// Cache-Control headers so intermediaries treat the endpoint as protected and
// non-cacheable. The handler is intended to protect /metrics in production
// while leaving the endpoint open in development and test environments.
func MetricsAuth(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		presented := bearerFromHeader(r.Header.Get("Authorization"))
		if !constantTimeTokenEqual(presented, token) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("WWW-Authenticate", "Bearer")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"Metrics authentication required"}}`))
			return
		}
		next(w, r)
	}
}

// constantTimeTokenEqual hashes both values first so ConstantTimeCompare
// always receives fixed-size inputs. This avoids the early length-mismatch
// return of comparing variable-length bearer strings directly.
func constantTimeTokenEqual(presented, expected string) bool {
	presentedHash := sha256.Sum256([]byte(presented))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(presentedHash[:], expectedHash[:]) == 1
}

// bearerFromHeader returns the token following a case-insensitive "Bearer"
// scheme in an Authorization header, or the empty string when the header is
// absent or malformed. Returning an empty value lets the caller feed it to a
// constant-time comparison without an early, length-leaking branch.
func bearerFromHeader(value string) string {
	trimmed := strings.TrimSpace(value)
	parts := strings.SplitN(trimmed, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// SetCleanupBacklog records the number of pending object-deletion jobs.
func (m *Metrics) SetCleanupBacklog(count int) { m.cleanupBacklog.Store(int64(count)) }

func formatUint(value uint64) string { return strconv.FormatUint(value, 10) }

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = randomID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				if logger != nil {
					logger.Error("panic recovered", "panic", value, "stack", string(debug.Stack()))
				}
				writeMiddlewareError(w, http.StatusInternalServerError, "internal_error", "Internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeMiddlewareError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":{"code":"` + code + `","message":"` + message + `"}}`))
}

func RequestLog(logger *slog.Logger, metrics *Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		wrapped := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(wrapped, r)
		if metrics != nil {
			metrics.Observe(wrapped.status)
		}
		if logger != nil {
			logger.Info("http request", "request_id", w.Header().Get("X-Request-ID"), "method", r.Method, "path", r.URL.Path, "status", wrapped.status, "duration_ms", time.Since(started).Milliseconds())
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *statusWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		if w.status == 0 {
			w.status = http.StatusOK
		}
		flusher.Flush()
	}
}
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijacking unsupported")
	}
	return hj.Hijack()
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102150405.000000000")
	}
	return hex.EncodeToString(b[:])
}

type requestIDKey struct{}
