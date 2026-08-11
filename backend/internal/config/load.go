package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// loader accumulates every parse failure so a single startup attempt reports
// all malformed variables instead of stopping at the first one.
type loader struct {
	problems []string
}

func (l *loader) fail(key, kind, value string) {
	l.problems = append(l.problems, fmt.Sprintf("%s must be a %s (got %q)", key, kind, value))
}

// stringValue applies its fallback only when a variable is absent. An explicit
// empty assignment remains empty so validation can reject required settings
// rather than silently substituting development credentials or endpoints.
// defaultPushEndpointAllowlist covers the canonical notification-delivery
// hosts of the four major push providers: Google FCM, Mozilla Push, Apple Web
// Push, and Windows. Operators may override it, but an explicit empty value is
// rejected in production when push is enabled.
const defaultPushEndpointAllowlist = "fcm.googleapis.com,push.services.mozilla.com,web-push.apple.com,wns.windows.com"

func (l *loader) stringValue(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func (l *loader) intValue(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		l.fail(key, "valid integer", value)
		return fallback
	}
	return parsed
}

func (l *loader) int32Value(key string, fallback int32) int32 {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		l.fail(key, "valid 32-bit integer", value)
		return fallback
	}
	return int32(parsed)
}

func (l *loader) int64Value(key string, fallback int64) int64 {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		l.fail(key, "valid integer", value)
		return fallback
	}
	return parsed
}

func (l *loader) uint64Value(key string, fallback uint64) uint64 {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		l.fail(key, "valid non-negative integer", value)
		return fallback
	}
	return parsed
}

func (l *loader) boolValue(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		l.fail(key, "boolean", value)
		return fallback
	}
	return parsed
}

func (l *loader) durationValue(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		l.fail(key, "valid duration", value)
		return fallback
	}
	return parsed
}

func (l *loader) urlValue(key, value string) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		l.fail(key, "valid http(s) URL", value)
	}
}

func (l *loader) cidrList(key string) []string {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := splitList(raw)
	for _, part := range parts {
		if _, err := netip.ParsePrefix(part); err != nil {
			l.fail(key, "CIDR", part)
		}
	}
	return parts
}

// rateLimitPolicies parses a comma-separated "type:limit/window" bucket list.
// Malformed entries are reported through the loader so startup surfaces every
// bad variable at once instead of silently weakening a policy.
func (l *loader) rateLimitPolicies(key, fallback string) []RateLimitBucket {
	raw := l.stringValue(key, fallback)
	var buckets []RateLimitBucket
	for _, part := range splitList(raw) {
		bucketType, limitAndWindow, ok := strings.Cut(part, ":")
		if !ok {
			l.fail(key, "bucket entries of the form type:limit/window", part)
			continue
		}
		limitRaw, windowRaw, ok := strings.Cut(limitAndWindow, "/")
		if !ok {
			l.fail(key, "bucket entries of the form type:limit/window", part)
			continue
		}
		limit, err := strconv.Atoi(strings.TrimSpace(limitRaw))
		if err != nil {
			l.fail(key, "valid integer limit", limitRaw)
			continue
		}
		window, err := time.ParseDuration(strings.TrimSpace(windowRaw))
		if err != nil {
			l.fail(key, "valid duration window", windowRaw)
			continue
		}
		bucketType = strings.TrimSpace(bucketType)
		if !isRateLimitBucketType(bucketType) {
			l.fail(key, "one of route, global, trustedIP, identity, user", bucketType)
			continue
		}
		buckets = append(buckets, RateLimitBucket{Type: bucketType, Limit: limit, Window: window})
	}
	return buckets
}

// Load reads and strictly parses every configuration variable. Defaults apply
// only to absent variables; malformed or explicitly invalid values are kept
// visible to the validation stage and reported together by LoadValidated.
func Load() (*Config, error) {
	l := &loader{}
	cfg := &Config{
		Environment:      normalizeEnvironment(l.stringValue("APP_ENV", EnvDevelopment)),
		Port:             l.stringValue("PORT", "8080"),
		PublicURL:        l.stringValue("PUBLIC_URL", "http://localhost:5173"),
		StorageDriver:    l.stringValue("STORAGE_DRIVER", ""),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		DatabaseMinConns: l.int32Value("DB_MIN_CONNS", 2),
		DatabaseMaxConns: l.int32Value("DB_MAX_CONNS", 10),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		AccessTokenTTL:   l.durationValue("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:  l.durationValue("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		VerificationTTL:  l.durationValue("VERIFICATION_TOKEN_TTL", 24*time.Hour),
		ResetTTL:         l.durationValue("RESET_TOKEN_TTL", time.Hour),
		PasswordHashCost: l.intValue("BCRYPT_COST", 12),

		SMTPHost:        os.Getenv("SMTP_HOST"),
		SMTPPort:        l.intValue("SMTP_PORT", 1025),
		SMTPUsername:    os.Getenv("SMTP_USERNAME"),
		SMTPPassword:    os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:        l.stringValue("SMTP_FROM", "no-reply@localhost"),
		SMTPTLS:         l.stringValue("SMTP_TLS", SMTPOff),
		SMTPDialTimeout: l.durationValue("SMTP_DIAL_TIMEOUT", 10*time.Second),
		SMTPTimeout:     l.durationValue("SMTP_TIMEOUT", 30*time.Second),

		S3Endpoint:     l.stringValue("S3_ENDPOINT", "http://localhost:9000"),
		S3Region:       l.stringValue("S3_REGION", "us-east-1"),
		S3Bucket:       l.stringValue("S3_BUCKET", "geoguessme-media"),
		S3AccessKey:    l.stringValue("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:    l.stringValue("S3_SECRET_KEY", "minioadmin"),
		S3UsePathStyle: l.boolValue("S3_USE_PATH_STYLE", true),

		AllowedOrigins:    splitList(l.stringValue("ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:3000")),
		TrustedProxyCIDRs: l.cidrList("TRUSTED_PROXY_CIDRS"),

		UploadMaxBytes:  l.int64Value("UPLOAD_MAX_BYTES", 10*1024*1024),
		AvatarMaxBytes:  l.int64Value("AVATAR_MAX_BYTES", 25*1024*1024),
		UploadMaxPixels: l.uint64Value("UPLOAD_MAX_PIXELS", 25_000_000),
		ChallengeTTL:    l.durationValue("CHALLENGE_TTL", 24*time.Hour),
		LocationHide:    l.durationValue("LOCATION_HIDE_DURATION", 48*time.Hour),
		ViewWindow:      l.durationValue("PHOTO_VIEW_WINDOW", 10*time.Second),
		PhotoRetention:  l.durationValue("PHOTO_RETENTION", 30*24*time.Hour),
		UploadDir:       l.stringValue("UPLOAD_DIR", "./uploads"),

		MediaProcessingWorker: l.boolValue("MEDIA_PROCESSING_WORKER", true),

		RateLimitRequests: l.intValue("RATE_LIMIT_REQUESTS", 10),
		RateLimitWindow:   l.durationValue("RATE_LIMIT_WINDOW", time.Minute),
		RateLimitPolicies: []RateLimitPolicy{
			{Name: "login", Buckets: l.rateLimitPolicies("RATE_LIMIT_LOGIN", "identity:10/1m,trustedIP:30/1m,global:300/1m")},
			{Name: "signup", Buckets: l.rateLimitPolicies("RATE_LIMIT_SIGNUP", "identity:3/1h,trustedIP:5/1h,global:60/1m")},
			{Name: "email", Buckets: l.rateLimitPolicies("RATE_LIMIT_EMAIL", "identity:3/1h,trustedIP:5/1h,global:30/1m")},
			{Name: "reset", Buckets: l.rateLimitPolicies("RATE_LIMIT_RESET", "trustedIP:10/1h")},
			{Name: "push", Buckets: l.rateLimitPolicies("RATE_LIMIT_PUSH", "user:10/1h,trustedIP:20/1h")},
			{Name: "default", Buckets: l.rateLimitPolicies("RATE_LIMIT_DEFAULT", "identity:10/1m,trustedIP:60/1m")},
		},
		RateLimitFailClosed: splitList(l.stringValue("RATE_LIMIT_FAIL_CLOSED", "login,signup,email,reset")),
		RateLimitStoreCap:   l.intValue("RATE_LIMIT_STORE_CAP", 50_000),
		LogLevel:            l.stringValue("LOG_LEVEL", "info"),
		MetricsToken:        strings.TrimSpace(os.Getenv("METRICS_TOKEN")),

		VapidPublicKey:  strings.TrimSpace(os.Getenv("VAPID_PUBLIC_KEY")),
		VapidPrivateKey: strings.TrimSpace(os.Getenv("VAPID_PRIVATE_KEY")),
		VapidSubject:    strings.TrimSpace(os.Getenv("VAPID_SUBJECT")),

		PushEndpointAllowlist: splitList(l.stringValue("PUSH_ENDPOINT_ALLOWLIST", defaultPushEndpointAllowlist)),

		PushMaxSubscriptionsPerUser: l.intValue("PUSH_MAX_SUBSCRIPTIONS_PER_USER", 5),
		PushSubscriptionExpiry:      l.durationValue("PUSH_SUBSCRIPTION_EXPIRY", 90*24*time.Hour),
		PushDeliveryWorkers:         l.intValue("PUSH_DELIVERY_WORKERS", 4),
		PushDeliveryPerHost:         l.intValue("PUSH_DELIVERY_PER_HOST", 2),
		PushDeliveryTimeout:         l.durationValue("PUSH_DELIVERY_TIMEOUT", 5*time.Second),
		PushQueueDepth:              l.intValue("PUSH_QUEUE_DEPTH", 256),
	}

	l.urlValue("PUBLIC_URL", cfg.PublicURL)
	l.urlValue("S3_ENDPOINT", cfg.S3Endpoint)
	if len(l.problems) > 0 {
		return cfg, errors.New(strings.Join(l.problems, "; "))
	}
	return cfg, nil
}

// LoadValidated combines parsing and semantic validation so startup reports
// every discoverable configuration problem in one attempt.
func LoadValidated() (*Config, error) {
	cfg, parseErr := Load()
	validateErr := cfg.Validate()
	switch {
	case parseErr != nil && validateErr != nil:
		return cfg, fmt.Errorf("%v; %v", parseErr, validateErr)
	case parseErr != nil:
		return cfg, parseErr
	default:
		return cfg, validateErr
	}
}
