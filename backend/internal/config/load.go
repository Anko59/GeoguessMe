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

		RateLimitRequests: l.intValue("RATE_LIMIT_REQUESTS", 10),
		RateLimitWindow:   l.durationValue("RATE_LIMIT_WINDOW", time.Minute),
		LogLevel:          l.stringValue("LOG_LEVEL", "info"),
		MetricsToken:      strings.TrimSpace(os.Getenv("METRICS_TOKEN")),

		VapidPublicKey:  strings.TrimSpace(os.Getenv("VAPID_PUBLIC_KEY")),
		VapidPrivateKey: strings.TrimSpace(os.Getenv("VAPID_PRIVATE_KEY")),
		VapidSubject:    strings.TrimSpace(os.Getenv("VAPID_SUBJECT")),
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
