package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
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
	UploadMaxPixels uint64
	ChallengeTTL    time.Duration
	ViewWindow      time.Duration
	PhotoRetention  time.Duration
	UploadDir       string

	RateLimitRequests int
	RateLimitWindow   time.Duration
	LogLevel          string
	MetricsToken      string
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

func Load() *Config {
	return &Config{
		Environment:      normalizeEnvironment(getEnv("APP_ENV", EnvDevelopment)),
		Port:             getEnv("PORT", "8080"),
		PublicURL:        getEnv("PUBLIC_URL", "http://localhost:5173"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		DatabaseMinConns: int32(getEnvAsInt("DB_MIN_CONNS", 2)),
		DatabaseMaxConns: int32(getEnvAsInt("DB_MAX_CONNS", 10)),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		AccessTokenTTL:   getEnvAsDuration("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:  getEnvAsDuration("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		VerificationTTL:  getEnvAsDuration("VERIFICATION_TOKEN_TTL", 24*time.Hour),
		ResetTTL:         getEnvAsDuration("RESET_TOKEN_TTL", time.Hour),
		PasswordHashCost: getEnvAsInt("BCRYPT_COST", 12),

		SMTPHost:        os.Getenv("SMTP_HOST"),
		SMTPPort:        getEnvAsInt("SMTP_PORT", 1025),
		SMTPUsername:    os.Getenv("SMTP_USERNAME"),
		SMTPPassword:    os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:        getEnv("SMTP_FROM", "no-reply@localhost"),
		SMTPTLS:         getEnv("SMTP_TLS", SMTPOff),
		SMTPDialTimeout: getEnvAsDuration("SMTP_DIAL_TIMEOUT", 10*time.Second),
		SMTPTimeout:     getEnvAsDuration("SMTP_TIMEOUT", 30*time.Second),

		S3Endpoint:     getEnv("S3_ENDPOINT", "http://localhost:9000"),
		S3Region:       getEnv("S3_REGION", "us-east-1"),
		S3Bucket:       getEnv("S3_BUCKET", "geoguessme-media"),
		S3AccessKey:    getEnv("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:    getEnv("S3_SECRET_KEY", "minioadmin"),
		S3UsePathStyle: getEnvAsBool("S3_USE_PATH_STYLE", true),

		AllowedOrigins:    splitList(getEnv("ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:3000")),
		TrustedProxyCIDRs: splitList(os.Getenv("TRUSTED_PROXY_CIDRS")),

		UploadMaxBytes:  getEnvAsInt64("UPLOAD_MAX_BYTES", 5*1024*1024),
		UploadMaxPixels: uint64(getEnvAsInt64("UPLOAD_MAX_PIXELS", 25_000_000)),
		ChallengeTTL:    getEnvAsDuration("CHALLENGE_TTL", 24*time.Hour),
		ViewWindow:      getEnvAsDuration("PHOTO_VIEW_WINDOW", 10*time.Second),
		PhotoRetention:  getEnvAsDuration("PHOTO_RETENTION", 30*24*time.Hour),
		UploadDir:       getEnv("UPLOAD_DIR", "./uploads"),

		RateLimitRequests: getEnvAsInt("RATE_LIMIT_REQUESTS", 10),
		RateLimitWindow:   getEnvAsDuration("RATE_LIMIT_WINDOW", time.Minute),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		MetricsToken:      strings.TrimSpace(os.Getenv("METRICS_TOKEN")),
	}
}

// Validate applies strict checks to every environment. Production enforces
// additional security constraints on top of the base rules.
func (c *Config) Validate() error {
	var problems []string

	switch c.Environment {
	case EnvDevelopment, EnvProduction, EnvTest:
	default:
		problems = append(problems, "APP_ENV must be one of development, production, test")
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
	if c.S3Endpoint == "" || c.S3Bucket == "" || c.S3AccessKey == "" || c.S3SecretKey == "" {
		problems = append(problems, "S3 endpoint, bucket, and credentials are required")
	}
	if _, err := url.Parse(c.S3Endpoint); err != nil || !strings.HasPrefix(c.S3Endpoint, "http") {
		problems = append(problems, "S3_ENDPOINT must be a valid http(s) URL")
	}
	if c.UploadMaxBytes <= 0 || c.UploadMaxPixels == 0 {
		problems = append(problems, "upload limits must be positive")
	}
	if c.ChallengeTTL <= 0 || c.ViewWindow <= 0 || c.PhotoRetention <= 0 {
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

	if strings.EqualFold(c.Environment, "production") {
		if c.SMTPHost == "" || c.SMTPFrom == "" {
			problems = append(problems, "SMTP_HOST and SMTP_FROM are required in production")
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

func LoadValidated() (*Config, error) {
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

// normalizeEnvironment trims surrounding whitespace and lower-cases the value
// so APP_ENV comparisons can be exact and case-insensitive at the same time.
func normalizeEnvironment(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func getEnvAsInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvAsInt64(key string, fallback int64) int64 {
	if value, ok := os.LookupEnv(key); ok {
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvAsBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvAsDuration(key string, fallback time.Duration) time.Duration {
	if value, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return fallback
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

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}
