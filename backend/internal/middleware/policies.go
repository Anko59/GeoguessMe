package middleware

import "time"

// DefaultPolicies returns the default rate-limit policies. These are the
// values mandated by the security remediation plan and are overridable through
// environment configuration (see config.RateLimitPolicies). FailClosed marks
// the expensive unauthenticated routes: they reject requests when the shared
// store has no capacity for a new bucket instead of silently allowing them.
func DefaultPolicies() []Policy {
	return []Policy{
		{
			Name: "login", FailClosed: true,
			Buckets: []BucketSpec{
				{Type: BucketIdentity, Limit: 10, Window: time.Minute},
				{Type: BucketTrustedIP, Limit: 30, Window: time.Minute},
				{Type: BucketGlobal, Limit: 300, Window: time.Minute},
			},
		},
		{
			Name: "signup", FailClosed: true,
			Buckets: []BucketSpec{
				{Type: BucketIdentity, Limit: 3, Window: time.Hour},
				{Type: BucketTrustedIP, Limit: 5, Window: time.Hour},
				{Type: BucketGlobal, Limit: 60, Window: time.Minute},
			},
		},
		{
			Name: "email", FailClosed: true,
			Buckets: []BucketSpec{
				{Type: BucketIdentity, Limit: 3, Window: time.Hour},
				{Type: BucketTrustedIP, Limit: 5, Window: time.Hour},
				{Type: BucketGlobal, Limit: 30, Window: time.Minute},
			},
		},
		{
			Name: "reset", FailClosed: true,
			Buckets: []BucketSpec{
				{Type: BucketTrustedIP, Limit: 10, Window: time.Hour},
			},
		},
		{
			Name: "push", FailClosed: false,
			Buckets: []BucketSpec{
				{Type: BucketUser, Limit: 10, Window: time.Hour},
				{Type: BucketTrustedIP, Limit: 20, Window: time.Hour},
			},
		},
		{
			Name: "default", FailClosed: false,
			Buckets: []BucketSpec{
				{Type: BucketIdentity, Limit: 10, Window: time.Minute},
				{Type: BucketTrustedIP, Limit: 60, Window: time.Minute},
			},
		},
	}
}

// PolicyByName returns the default policy with the given name.
func PolicyByName(name string) (Policy, bool) {
	for _, p := range DefaultPolicies() {
		if p.Name == name {
			return p, true
		}
	}
	return Policy{}, false
}
