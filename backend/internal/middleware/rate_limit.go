package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// cleanupInterval is how often stale counters are pruned from the shared
// store. Expired counters are also reclaimed lazily on access, so this
// interval only bounds how long a never-touched counter lingers.
const cleanupInterval = 10 * time.Minute

// maxIdentityBodyBytes bounds the request bodies inspected for an identity.
// Larger or non-JSON bodies are never consumed and fall back to IP-based keys.
const maxIdentityBodyBytes = 64 * 1024

// keyExtractors provides the per-bucket request keys. A nil extractor or an
// empty key skips that bucket for the request.
type keyExtractors struct {
	route     func(*http.Request) string
	trustedIP func(*http.Request) string
	identity  func(*http.Request) string
	user      func(*http.Request) string
}

// keys builds the key map for one request. The global bucket is process-wide
// per policy, so a constant key suffices; it is only used when a policy
// declares a global bucket.
func (e keyExtractors) keys(r *http.Request) map[BucketType]string {
	keys := make(map[BucketType]string, 5)
	if e.route != nil {
		if k := e.route(r); k != "" {
			keys[BucketRoute] = k
		}
	}
	if e.trustedIP != nil {
		if k := e.trustedIP(r); k != "" {
			keys[BucketTrustedIP] = k
		}
	}
	if e.identity != nil {
		if k := e.identity(r); k != "" {
			keys[BucketIdentity] = k
		}
	}
	if e.user != nil {
		if k := e.user(r); k != "" {
			keys[BucketUser] = k
		}
	}
	keys[BucketGlobal] = "global"
	return keys
}

// newPolicyMiddleware builds an HTTP middleware that enforces the policy
// against the shared bounded store.
func newPolicyMiddleware(p Policy, extractors keyExtractors) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, retryAfter := limiterStore.allow(p, extractors.keys(r))
			if !allowed {
				writeRateLimited(w, retryAfter)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimit creates a rate limiting middleware that uses the client IP
// address (including X-Forwarded-For without proxy validation for legacy
// callers) as the rate-limit key.
func RateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	return newPolicyMiddleware(
		Policy{Name: "default", Buckets: []BucketSpec{{Type: BucketTrustedIP, Limit: limit, Window: window}}},
		keyExtractors{trustedIP: legacyClientKey},
	)
}

// RateLimitWithTrustedProxies only accepts forwarded client IP headers when
// the immediate peer is in a configured proxy network.
func RateLimitWithTrustedProxies(limit int, window time.Duration, trustedCIDRs []string) func(http.Handler) http.Handler {
	return newPolicyMiddleware(
		Policy{Name: "default", Buckets: []BucketSpec{{Type: BucketTrustedIP, Limit: limit, Window: window}}},
		keyExtractors{trustedIP: func(r *http.Request) string { return clientKey(r, trustedCIDRs) }},
	)
}

// RateLimitByIdentity rate-limits by the trusted client IP combined with the
// identity field (username or email) extracted from small JSON request bodies.
// Non-JSON and larger requests use the client-IP limiter without consuming the
// body. The returned middleware keeps the pre-F-04 behavior: one trusted-IP
// bucket and one identity bucket, both with the given limit and window.
func RateLimitByIdentity(limit int, window time.Duration, trustedCIDRs []string) func(http.Handler) http.Handler {
	p := Policy{
		Name: "identity",
		Buckets: []BucketSpec{
			{Type: BucketTrustedIP, Limit: limit, Window: window},
			{Type: BucketIdentity, Limit: limit, Window: window},
		},
	}
	ex := keyExtractors{
		trustedIP: func(r *http.Request) string { return clientKey(r, trustedCIDRs) },
		identity: func(r *http.Request) string {
			return clientKey(r, trustedCIDRs) + "|" + extractIdentity(r)
		},
	}
	return newPolicyMiddleware(p, ex)
}

// extractIdentity returns the normalized username or email from a small JSON
// request body, or the empty string when the body is not a small JSON object.
// The body is always restored so downstream handlers see the original payload.
// ExtractIdentity returns the normalized username or email from a small JSON
// request body, or the empty string. It is exported so the wiring layer can
// compose the body identity with the authenticated user for protected routes
// whose requests carry no identity fields (for example verification-email
// sends, which identify the caller through the auth context instead).
func ExtractIdentity(r *http.Request) string {
	return extractIdentity(r)
}

func extractIdentity(r *http.Request) string {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" || r.ContentLength < 0 || r.ContentLength > maxIdentityBodyBytes {
		return ""
	}
	var body []byte
	var readErr error
	if r.Body != nil {
		body, readErr = io.ReadAll(r.Body)
	}
	if readErr == nil {
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
	return identity
}

// PolicyOptions configures PolicyMiddleware's request-key extractors.
type PolicyOptions struct {
	// TrustedCIDRs are the proxy networks whose forwarded client headers are
	// trusted when deriving the client IP (see clientKey).
	TrustedCIDRs []string
	// Identity overrides the default body-identity extractor. When non-nil it
	// replaces extractIdentity entirely, so callers compose the body identity
	// with the authenticated user for protected routes.
	Identity func(*http.Request) string
	// User supplies the authenticated-user key for policies that declare a
	// user bucket. It may be nil when no wired policy has one.
	User func(*http.Request) string
}

// PolicyMiddleware builds an HTTP middleware that enforces p against the
// shared bounded store. Keys are derived from the trusted client IP, the
// request identity (default: username/email from small JSON bodies), and the
// authenticated user supplied through opts.User. The two-phase check-then-
// increment semantics of the engine apply unchanged: rejected requests never
// consume quota.
func PolicyMiddleware(p Policy, opts PolicyOptions) func(http.Handler) http.Handler {
	identity := extractIdentity
	if opts.Identity != nil {
		identity = opts.Identity
	}
	ex := keyExtractors{
		route: func(r *http.Request) string {
			return r.Method + " " + r.Pattern
		},
		trustedIP: func(r *http.Request) string { return clientKey(r, opts.TrustedCIDRs) },
		identity:  identity,
		user:      opts.User,
	}
	return newPolicyMiddleware(p, ex)
}

func writeRateLimited(w http.ResponseWriter, retryAfter time.Duration) {
	w.Header().Set("Retry-After", retryAfterHeader(retryAfter))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"error":{"code":"rate_limited","message":"Too many requests"}}`))
}

// retryAfterHeader returns an integer-second Retry-After value per RFC 9110.
func retryAfterHeader(retryAfter time.Duration) string {
	seconds := int64(retryAfter.Seconds())
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
	limiterStore.mu.Lock()
	defer limiterStore.mu.Unlock()
	if fn == nil {
		limiterStore.clock = time.Now
		limiterStore.offset = 0
		return
	}
	limiterStore.clock = fn
}

// ResetRateLimiter clears every tracked counter and restores the real-time
// clock so tests can start from a clean slate. Production code must not
// call it.
func ResetRateLimiter() {
	limiterStore.mu.Lock()
	defer limiterStore.mu.Unlock()
	limiterStore.buckets = make(map[string]*counter)
	limiterStore.clock = time.Now
	limiterStore.offset = 0
	limiterStore.rejected = 0
	limiterStore.rejectedByPolicy = make(map[string]int64)
	limiterStore.capacity = maxRateLimitBuckets
}

// AdvanceTestClock moves the rate-limiter clock forward by d. When the test
// clock has never been set the call anchors the simulated clock at the
// current wall-clock time and then advances it. Production code must not
// call this.
func AdvanceTestClock(d time.Duration) {
	limiterStore.mu.Lock()
	defer limiterStore.mu.Unlock()

	// On the first call (or first after reset), anchor to wall-clock time.
	if limiterStore.offset == 0 {
		limiterStore.start = time.Now()
	}
	limiterStore.offset += d
	limiterStore.clock = func() time.Time { return limiterStore.start.Add(limiterStore.offset) }
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
