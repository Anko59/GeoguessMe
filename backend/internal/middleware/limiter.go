package middleware

import (
	"log/slog"
	"strconv"
	"sync"
	"time"
)

// BucketType identifies which request dimension a rate-limit bucket counts.
type BucketType string

// Supported bucket types. A policy combines one or more of these; a request is
// allowed only while every applicable bucket has remaining capacity.
const (
	BucketRoute     BucketType = "route"
	BucketGlobal    BucketType = "global"
	BucketTrustedIP BucketType = "trustedIP"
	BucketIdentity  BucketType = "identity"
	BucketUser      BucketType = "user"
)

// ValidBucketType reports whether t is a supported bucket type.
func ValidBucketType(t BucketType) bool {
	switch t {
	case BucketRoute, BucketGlobal, BucketTrustedIP, BucketIdentity, BucketUser:
		return true
	default:
		return false
	}
}

// BucketSpec defines one fixed-window counter for a policy.
type BucketSpec struct {
	Type   BucketType
	Limit  int
	Window time.Duration
}

// Policy is a named set of rate-limit buckets. A request is allowed only when
// every applicable bucket has capacity. FailClosed policies reject requests
// outright when the shared store cannot create a new bucket because its
// capacity is exhausted; these are the expensive unauthenticated routes
// (login, signup, verification/recovery email, password reset).
type Policy struct {
	Name       string
	Buckets    []BucketSpec
	FailClosed bool
}

// counter is a single fixed-window counter for one policy+bucket-type+key.
type counter struct {
	count   int
	resetAt time.Time
}

// maxRateLimitBuckets bounds the shared in-process store. The backend runs as
// a single replica, so the store is authoritative process-wide; the bound
// prevents an attacker from growing the store without limit. Expired counters
// are reclaimed by the periodic sweeper and lazily on access.
const maxRateLimitBuckets = 50_000

// bucketStore is the shared, bounded store of rate-limit counters. Every
// policy and middleware instance draws from the same store, which preserves
// the documented single-backend-replica invariant: limits are enforced
// process-wide and are exact only while the backend has one replica.
type bucketStore struct {
	mu               sync.Mutex
	buckets          map[string]*counter
	clock            func() time.Time
	offset           time.Duration
	start            time.Time
	rejected         int64
	rejectedByPolicy map[string]int64
}

// limiterStore is the process-wide bucket store used by every middleware
// instance. Tests reset it through ResetRateLimiter.
var limiterStore = newBucketStore()

func newBucketStore() *bucketStore {
	s := &bucketStore{
		buckets:          make(map[string]*counter),
		clock:            time.Now,
		rejectedByPolicy: make(map[string]int64),
	}
	go s.sweepLoop()
	return s
}

// sweepLoop periodically removes expired counters (TTL eviction).
func (s *bucketStore) sweepLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		s.sweepLocked(s.clock())
		s.mu.Unlock()
	}
}

// sweepLocked deletes every counter whose window has elapsed. Callers hold s.mu.
func (s *bucketStore) sweepLocked(now time.Time) {
	for key, c := range s.buckets {
		if !now.Before(c.resetAt) {
			delete(s.buckets, key)
		}
	}
}

// evictExpiringLocked removes the soonest-expiring counter from a bounded
// sample to make room for a new key. It runs only for non-fail-closed policies
// when the store is at capacity, so the linear scan is bounded and rare.
// Callers hold s.mu.
func (s *bucketStore) evictExpiringLocked() {
	const sample = 256
	var victim string
	var victimReset time.Time
	scanned := 0
	for key, c := range s.buckets {
		if victim == "" || c.resetAt.Before(victimReset) {
			victim = key
			victimReset = c.resetAt
		}
		scanned++
		if scanned >= sample {
			break
		}
	}
	if victim != "" {
		delete(s.buckets, victim)
	}
}

// compositeKey builds the store key for one bucket. The limit and window are
// part of the key so two middleware instances that share a policy name but use
// different limits never read each other's counters.
func compositeKey(p Policy, spec BucketSpec, key string) string {
	return p.Name + ":" + string(spec.Type) + ":" +
		strconv.Itoa(spec.Limit) + "/" + spec.Window.String() + ":" + key
}

// allow evaluates every bucket of the policy for the request's keys. It
// returns whether the request is permitted and, when rejected, how long until
// the limiting bucket resets. A rejected request never consumes quota on any
// bucket: capacity is checked across all buckets first, and counters are only
// incremented once every bucket has passed.
func (s *bucketStore) allow(p Policy, keys map[BucketType]string) (allowed bool, retryAfter time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock()
	// Phase 1: materialize every applicable bucket and check capacity. No
	// counter is mutated here.
	type admitted struct {
		c *counter
	}
	var pending []admitted
	var earliest time.Duration
	for _, spec := range p.Buckets {
		key := keys[spec.Type]
		if key == "" || spec.Limit <= 0 || spec.Window <= 0 {
			continue
		}
		fullKey := compositeKey(p, spec, key)
		c, ok := s.buckets[fullKey]
		if !ok || !now.Before(c.resetAt) {
			// Lazy TTL: an expired counter is reclaimed before reuse.
			if ok {
				delete(s.buckets, fullKey)
			}
			if len(s.buckets) >= maxRateLimitBuckets {
				s.sweepLocked(now)
				if len(s.buckets) >= maxRateLimitBuckets {
					if p.FailClosed {
						s.countRejectionLocked(p.Name)
						slog.Default().Warn("rate limit capacity exhausted; rejecting request",
							"policy", p.Name)
						return false, retryAfterDuration(now, now.Add(spec.Window))
					}
					s.evictExpiringLocked()
				}
			}
			c = &counter{count: 0, resetAt: now.Add(spec.Window)}
			s.buckets[fullKey] = c
		}
		if c.count >= spec.Limit {
			s.countRejectionLocked(p.Name)
			ra := retryAfterDuration(now, c.resetAt)
			if earliest == 0 || ra < earliest {
				earliest = ra
			}
			continue
		}
		pending = append(pending, admitted{c: c})
	}
	if earliest > 0 {
		return false, earliest
	}
	// Phase 2: every bucket has capacity, so the request is admitted and
	// consumes one unit from each.
	for _, b := range pending {
		b.c.count++
	}
	return true, 0
}

func (s *bucketStore) countRejectionLocked(policy string) {
	s.rejected++
	s.rejectedByPolicy[policy]++
}

// retryAfterDuration returns how long until resetAt, rounded up to a whole
// number of seconds with a one-second floor (RFC 9110 Retry-After). Rounding
// up is required: advising a client to retry before the counter resets would
// let it burst early.
func retryAfterDuration(now, resetAt time.Time) time.Duration {
	remaining := resetAt.Sub(now)
	seconds := int64((remaining + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return time.Duration(seconds) * time.Second
}

// Rejections returns the total number of 429 responses emitted by the limiter
// since the last reset. F-08 surfaces this as a Prometheus metric.
func Rejections() int64 {
	limiterStore.mu.Lock()
	defer limiterStore.mu.Unlock()
	return limiterStore.rejected
}

// RejectionsByPolicy returns a snapshot of rejection counts per policy.
func RejectionsByPolicy() map[string]int64 {
	limiterStore.mu.Lock()
	defer limiterStore.mu.Unlock()
	out := make(map[string]int64, len(limiterStore.rejectedByPolicy))
	for policy, count := range limiterStore.rejectedByPolicy {
		out[policy] = count
	}
	return out
}

// StoreSize returns the number of live rate-limit counters. It exists for
// tests and capacity observability.
func StoreSize() int {
	limiterStore.mu.Lock()
	defer limiterStore.mu.Unlock()
	return len(limiterStore.buckets)
}
