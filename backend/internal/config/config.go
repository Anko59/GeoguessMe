package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Config is the complete runtime configuration. Values are deliberately kept
// in one typed struct so handlers and background workers cannot read process
// environment variables independently.
type Config struct {
	Environment string
	Port        string
	PublicURL   string

	// StorageDriver selects the object store. "local" uses the filesystem via
	// UploadDir; "s3" or the absent-value default uses S3-compatible storage.
	StorageDriver string

	DatabaseURL      string
	DatabaseMinConns int32
	DatabaseMaxConns int32

	JWTSecret        string
	AccessTokenTTL   time.Duration
	RefreshTokenTTL  time.Duration
	VerificationTTL  time.Duration
	ResetTTL         time.Duration
	PasswordHashCost int

	SMTPHost        string
	SMTPPort        int
	SMTPUsername    string
	SMTPPassword    string
	SMTPFrom        string
	SMTPTLS         string
	SMTPDialTimeout time.Duration
	SMTPTimeout     time.Duration

	S3Endpoint     string
	S3Region       string
	S3Bucket       string
	S3AccessKey    string
	S3SecretKey    string
	S3UsePathStyle bool

	AllowedOrigins    []string
	TrustedProxyCIDRs []string

	UploadMaxBytes  int64
	AvatarMaxBytes  int64
	UploadMaxPixels uint64
	ChallengeTTL    time.Duration
	LocationHide    time.Duration
	ViewWindow      time.Duration
	PhotoRetention  time.Duration
	UploadDir       string

	RateLimitRequests int
	RateLimitWindow   time.Duration

	// RateLimitPolicies are the per-route multi-bucket rate-limit policies.
	// Each bucket is expressed as "type:limit/window" where type is one of
	// route, global, trustedIP, identity, user. RateLimitFailClosed names the
	// policies that reject requests when the shared bucket store is exhausted
	// (the expensive unauthenticated routes). RateLimitStoreCap bounds the
	// shared in-process bucket store.
	RateLimitPolicies   []RateLimitPolicy
	RateLimitFailClosed []string
	RateLimitStoreCap   int
	LogLevel            string
	MetricsToken        string

	// VAPID keys (base64url) enable Web Push. A complete keypair and contact
	// subject are required when push is enabled. Production may leave all three
	// unset to disable push explicitly while the rest of the application runs.
	VapidPublicKey  string
	VapidPrivateKey string
	VapidSubject    string
}

// RateLimitBucket is one fixed-window counter of a rate-limit policy.
// Type is one of route, global, trustedIP, identity, user.
type RateLimitBucket struct {
	Type   string
	Limit  int
	Window time.Duration
}

// RateLimitPolicy is a named set of rate-limit buckets.
type RateLimitPolicy struct {
	Name    string
	Buckets []RateLimitBucket
}

// SMTP modes.
const (
	SMTPOff      = "off"
	SMTPStartTLS = "starttls"
	SMTPTLS      = "tls"
)

// Supported APP_ENV values. Environment is normalized to one of these before
// validation, so every downstream comparison can be exact instead of relying
// on case-insensitive matching scattered across the codebase.
const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
	EnvTest        = "test"
)

// minMetricsTokenBytes is the minimum accepted length for a production
// metrics bearer token. 32 bytes resists brute force while staying compact
// enough for an HTTP header.
const minMetricsTokenBytes = 32

// Validate applies strict checks to every environment. Production enforces
// additional security constraints on top of the base rules.
func (c *Config) Validate() error {
	var problems []string

	switch c.Environment {
	case EnvDevelopment, EnvProduction, EnvTest:
	default:
		problems = append(problems, "APP_ENV must be one of development, production, test")
	}
	switch strings.ToLower(c.StorageDriver) {
	case "", "s3", "local":
	default:
		problems = append(problems, "STORAGE_DRIVER must be one of s3, local")
	}
	if port, err := strconv.Atoi(strings.TrimSpace(c.Port)); err != nil || port < 1 || port > 65535 {
		problems = append(problems, "PORT must be an integer between 1 and 65535")
	}
	if c.DatabaseURL == "" {
		problems = append(problems, "DATABASE_URL is required")
	}
	if len(c.JWTSecret) < 32 {
		problems = append(problems, "JWT_SECRET must be at least 32 characters")
	}
	if c.AccessTokenTTL <= 0 || c.RefreshTokenTTL <= 0 || c.VerificationTTL <= 0 || c.ResetTTL <= 0 {
		problems = append(problems, "token lifetimes must be positive")
	}
	if c.AccessTokenTTL >= c.RefreshTokenTTL {
		problems = append(problems, "ACCESS_TOKEN_TTL must be shorter than REFRESH_TOKEN_TTL")
	}
	if c.PasswordHashCost < 4 || c.PasswordHashCost > 31 {
		problems = append(problems, "BCRYPT_COST must be between 4 and 31")
	}
	if c.DatabaseMinConns < 0 {
		problems = append(problems, "DB_MIN_CONNS must not be negative")
	}
	if c.DatabaseMaxConns < 1 || c.DatabaseMaxConns < c.DatabaseMinConns {
		problems = append(problems, "DB_MAX_CONNS must be at least 1 and at least DB_MIN_CONNS")
	}
	if len(c.AllowedOrigins) == 0 || contains(c.AllowedOrigins, "*") {
		problems = append(problems, "ALLOWED_ORIGINS must contain explicit origins")
	}
	for _, origin := range c.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme == "" || u.Host == "" {
			problems = append(problems, fmt.Sprintf("invalid browser origin %q", origin))
		}
	}
	for _, cidr := range c.TrustedProxyCIDRs {
		if _, err := netip.ParsePrefix(cidr); err != nil {
			problems = append(problems, fmt.Sprintf("invalid trusted proxy CIDR %q", cidr))
		}
	}
	if c.S3Endpoint == "" || c.S3Bucket == "" || c.S3AccessKey == "" || c.S3SecretKey == "" {
		problems = append(problems, "S3 endpoint, bucket, and credentials are required")
	}
	s3URL, s3Err := url.Parse(strings.TrimSpace(c.S3Endpoint))
	if s3Err != nil || s3URL.Host == "" || (s3URL.Scheme != "http" && s3URL.Scheme != "https") {
		problems = append(problems, "S3_ENDPOINT must be a valid http(s) URL")
	}
	publicURL, publicErr := url.Parse(strings.TrimSpace(c.PublicURL))
	if publicErr != nil || publicURL.Host == "" || (publicURL.Scheme != "http" && publicURL.Scheme != "https") {
		problems = append(problems, "PUBLIC_URL must be a valid http(s) URL")
	}
	if c.UploadMaxBytes <= 0 || c.AvatarMaxBytes <= 0 || c.UploadMaxPixels == 0 {
		problems = append(problems, "upload limits must be positive")
	}
	if c.ChallengeTTL <= 0 || c.ViewWindow <= 0 || c.LocationHide <= 0 || c.PhotoRetention <= 0 {
		problems = append(problems, "challenge timing values must be positive")
	}
	if c.ViewWindow >= c.ChallengeTTL {
		problems = append(problems, "PHOTO_VIEW_WINDOW must be shorter than CHALLENGE_TTL")
	}
	if c.PhotoRetention < c.ChallengeTTL {
		problems = append(problems, "PHOTO_RETENTION must be at least CHALLENGE_TTL")
	}
	if c.RateLimitRequests <= 0 || c.RateLimitWindow <= 0 {
		problems = append(problems, "rate limit values must be positive")
	}
	if c.RateLimitStoreCap < 1 {
		problems = append(problems, "RATE_LIMIT_STORE_CAP must be at least 1")
	}
	if len(c.RateLimitPolicies) == 0 {
		problems = append(problems, "at least one rate limit policy is required")
	}
	knownPolicies := make(map[string]bool, len(c.RateLimitPolicies))
	for _, p := range c.RateLimitPolicies {
		if p.Name == "" {
			problems = append(problems, "rate limit policy name must not be empty")
			continue
		}
		if knownPolicies[p.Name] {
			problems = append(problems, fmt.Sprintf("duplicate rate limit policy %q", p.Name))
		}
		knownPolicies[p.Name] = true
		if len(p.Buckets) == 0 {
			problems = append(problems, fmt.Sprintf("rate limit policy %q has no buckets", p.Name))
		}
		knownBucketTypes := make(map[string]bool, len(p.Buckets))
		for _, b := range p.Buckets {
			if knownBucketTypes[b.Type] {
				problems = append(problems, fmt.Sprintf("rate limit policy %q has duplicate bucket type %q", p.Name, b.Type))
			}
			knownBucketTypes[b.Type] = true
			if b.Limit <= 0 {
				problems = append(problems, fmt.Sprintf("rate limit policy %q bucket %q must have a positive limit", p.Name, b.Type))
			}
			if b.Window <= 0 {
				problems = append(problems, fmt.Sprintf("rate limit policy %q bucket %q must have a positive window", p.Name, b.Type))
			}
			if !isRateLimitBucketType(b.Type) {
				problems = append(problems, fmt.Sprintf("rate limit policy %q has unknown bucket type %q", p.Name, b.Type))
			}
		}
	}
	for _, name := range c.RateLimitFailClosed {
		if !knownPolicies[name] {
			problems = append(problems, fmt.Sprintf("rate limit fail-closed policy %q is not a known policy", name))
		}
	}
	if c.SMTPDialTimeout <= 0 || c.SMTPTimeout <= 0 {
		problems = append(problems, "SMTP timeouts must be positive")
	}

	switch strings.ToLower(c.SMTPTLS) {
	case SMTPOff, SMTPStartTLS, SMTPTLS:
	default:
		problems = append(problems, "SMTP_TLS must be one of off, starttls, tls")
	}
	if c.SMTPHost != "" && (c.SMTPPort < 1 || c.SMTPPort > 65535) {
		problems = append(problems, "SMTP_PORT must be an integer between 1 and 65535")
	}
	// VAPID is opt-in in production and ephemeral in development/test. A partial
	// configuration must never be interpreted as either mode.
	hasVapidPublicKey := c.VapidPublicKey != ""
	hasVapidPrivateKey := c.VapidPrivateKey != ""
	hasVapidKeyPair := hasVapidPublicKey && hasVapidPrivateKey
	if hasVapidPublicKey != hasVapidPrivateKey {
		problems = append(problems, "VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY must be provided together")
	}
	if hasVapidKeyPair && !isVapidSubject(c.VapidSubject) {
		problems = append(problems, "VAPID_SUBJECT must be a mailto: or https: URL when VAPID keys are configured")
	}
	if !hasVapidKeyPair && c.VapidSubject != "" {
		problems = append(problems, "VAPID_SUBJECT requires VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY")
	}

	if strings.EqualFold(c.Environment, "production") {
		if c.SMTPHost == "" || c.SMTPFrom == "" {
			problems = append(problems, "SMTP_HOST and SMTP_FROM are required in production")
		}
		if (c.SMTPUsername == "") != (c.SMTPPassword == "") {
			problems = append(problems, "SMTP_USERNAME and SMTP_PASSWORD must be provided together")
		}
		if strings.EqualFold(c.SMTPTLS, SMTPOff) {
			problems = append(problems, "SMTP_TLS cannot be off in production")
		}
		// Authenticated credentials must never travel over a plaintext link.
		if c.SMTPUsername != "" && strings.EqualFold(c.SMTPTLS, SMTPOff) {
			problems = append(problems, "authenticated SMTP requires SMTP_TLS starttls or tls")
		}
		if strings.HasPrefix(c.S3Endpoint, "http://localhost") {
			problems = append(problems, "production storage must not use local MinIO")
		}
		if s3URL.Scheme != "https" {
			problems = append(problems, "S3_ENDPOINT must use HTTPS in production")
		}
		if publicURL.Scheme != "https" {
			problems = append(problems, "PUBLIC_URL must use HTTPS in production")
		}
		if c.MetricsToken == "" {
			problems = append(problems, "METRICS_TOKEN is required in production")
		} else if len(c.MetricsToken) < minMetricsTokenBytes {
			problems = append(problems, "METRICS_TOKEN must be at least 32 bytes in production")
		}
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

// IsTest reports whether the validated environment is the test environment.
// Test-only control endpoints are registered only when this is true, keeping
// them behind the validated environment gate.
func (c *Config) IsTest() bool { return c.Environment == EnvTest }

// MetricsAuthRequired reports whether the /metrics endpoint must authenticate
// callers. Authentication is required unless the environment is explicitly
// development or test, so production (and any value that survives validation
// other than those two) is protected by default.
func (c *Config) MetricsAuthRequired() bool {
	return c.Environment != EnvDevelopment && c.Environment != EnvTest
}

// isVapidSubject reports whether value is an acceptable RFC 8292 VAPID contact:
// a non-empty mailto address or HTTPS URL with a host.
func isVapidSubject(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "mailto":
		return parsed.Opaque != ""
	case "https":
		return parsed.Host != ""
	default:
		return false
	}
}

// normalizeEnvironment trims surrounding whitespace and lower-cases the value
// so APP_ENV comparisons can be exact and case-insensitive at the same time.
func normalizeEnvironment(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func splitList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// isRateLimitBucketType reports whether t is a supported rate-limit bucket
// type. The middleware package owns the canonical BucketType constants; this
// helper keeps the strict loader self-contained without importing middleware.
func isRateLimitBucketType(t string) bool {
	switch t {
	case "route", "global", "trustedIP", "identity", "user":
		return true
	default:
		return false
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}
