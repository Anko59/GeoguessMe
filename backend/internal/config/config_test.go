package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	// Clear relevant environment variables to ensure defaults are used
	os.Unsetenv("PORT")
	os.Unsetenv("DATABASE_URL")
	os.Unsetenv("JWT_SECRET")
	os.Unsetenv("ALLOWED_ORIGINS")
	os.Unsetenv("RATE_LIMIT_REQUESTS")
	os.Unsetenv("RATE_LIMIT_WINDOW")
	os.Unsetenv("UPLOAD_DIR")

	cfg := Load()

	if cfg.Port != "8080" {
		t.Errorf("Expected default Port 8080, got %s", cfg.Port)
	}
	if cfg.RateLimitRequests != 10 {
		t.Errorf("Expected default RateLimitRequests 10, got %d", cfg.RateLimitRequests)
	}
	if cfg.RateLimitWindow != time.Minute {
		t.Errorf("Expected default RateLimitWindow 1m, got %v", cfg.RateLimitWindow)
	}
	if cfg.UploadDir != "./uploads" {
		t.Errorf("Expected default UploadDir ./uploads, got %s", cfg.UploadDir)
	}
	if len(cfg.AllowedOrigins) != 2 {
		t.Errorf("Expected 2 default AllowedOrigins, got %d", len(cfg.AllowedOrigins))
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	os.Setenv("PORT", "9090")
	os.Setenv("DATABASE_URL", "postgres://test:test@localhost:5432/test")
	os.Setenv("JWT_SECRET", "testsecret")
	os.Setenv("ALLOWED_ORIGINS", "http://example.com,http://test.com")
	os.Setenv("RATE_LIMIT_REQUESTS", "100")
	os.Setenv("RATE_LIMIT_WINDOW", "1h")
	os.Setenv("UPLOAD_DIR", "/tmp/uploads")

	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("ALLOWED_ORIGINS")
		os.Unsetenv("RATE_LIMIT_REQUESTS")
		os.Unsetenv("RATE_LIMIT_WINDOW")
		os.Unsetenv("UPLOAD_DIR")
	}()

	cfg := Load()

	if cfg.Port != "9090" {
		t.Errorf("Expected Port 9090, got %s", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://test:test@localhost:5432/test" {
		t.Errorf("Expected DatabaseURL postgres://test:test@localhost:5432/test, got %s", cfg.DatabaseURL)
	}
	if cfg.JWTSecret != "testsecret" {
		t.Errorf("Expected JWTSecret testsecret, got %s", cfg.JWTSecret)
	}
	if len(cfg.AllowedOrigins) != 2 || cfg.AllowedOrigins[0] != "http://example.com" {
		t.Errorf("Expected AllowedOrigins [http://example.com http://test.com], got %v", cfg.AllowedOrigins)
	}
	if cfg.RateLimitRequests != 100 {
		t.Errorf("Expected RateLimitRequests 100, got %d", cfg.RateLimitRequests)
	}
	if cfg.RateLimitWindow != time.Hour {
		t.Errorf("Expected RateLimitWindow 1h, got %v", cfg.RateLimitWindow)
	}
	if cfg.UploadDir != "/tmp/uploads" {
		t.Errorf("Expected UploadDir /tmp/uploads, got %s", cfg.UploadDir)
	}
}

func validConfig() *Config {
	return &Config{
		Environment:       "development",
		Port:              "8080",
		PublicURL:         "http://localhost:5173",
		DatabaseURL:       "postgres://u:p@localhost/db?sslmode=disable",
		DatabaseMinConns:  2,
		DatabaseMaxConns:  10,
		JWTSecret:         "a-valid-secret-that-is-at-least-32-bytes-long",
		AccessTokenTTL:    15 * time.Minute,
		RefreshTokenTTL:   30 * 24 * time.Hour,
		VerificationTTL:   24 * time.Hour,
		ResetTTL:          time.Hour,
		PasswordHashCost:  10,
		SMTPHost:          "localhost",
		SMTPPort:          1025,
		SMTPFrom:          "no-reply@localhost",
		SMTPTLS:           "off",
		SMTPDialTimeout:   10 * time.Second,
		SMTPTimeout:       30 * time.Second,
		S3Endpoint:        "http://localhost:9000",
		S3Bucket:          "media",
		S3AccessKey:       "k",
		S3SecretKey:       "s",
		AllowedOrigins:    []string{"http://localhost:5173"},
		UploadMaxBytes:    5 * 1024 * 1024,
		UploadMaxPixels:   25_000_000,
		ChallengeTTL:      24 * time.Hour,
		ViewWindow:        10 * time.Second,
		PhotoRetention:    30 * 24 * time.Hour,
		RateLimitRequests: 10,
		RateLimitWindow:   time.Minute,
	}
}

func TestValidateAcceptsValidDevelopmentConfig(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}
}

func TestValidateRejectsMisconfiguration(t *testing.T) {
	cases := map[string]func(*Config){
		"port out of range":          func(c *Config) { c.Port = "99999" },
		"access longer than refresh": func(c *Config) { c.AccessTokenTTL = c.RefreshTokenTTL + time.Hour },
		"weak bcrypt cost":           func(c *Config) { c.PasswordHashCost = 2 },
		"max conns below min":        func(c *Config) { c.DatabaseMaxConns = 1; c.DatabaseMinConns = 5 },
		"wildcard origin":            func(c *Config) { c.AllowedOrigins = []string{"*"} },
		"view window not shorter":    func(c *Config) { c.ViewWindow = c.ChallengeTTL },
		"retention below challenge":  func(c *Config) { c.PhotoRetention = c.ChallengeTTL / 2 },
		"unknown smtp tls":           func(c *Config) { c.SMTPHost = "smtp.example"; c.SMTPTLS = "ssl" },
		"zero rate window":           func(c *Config) { c.RateLimitWindow = 0 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := validConfig()
			mutate(c)
			if err := c.Validate(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestValidateProductionEnforcesSMTPAndStorage(t *testing.T) {
	c := validConfig()
	c.Environment = "production"
	c.PublicURL = "https://app.example.test"
	c.SMTPTLS = "off"
	c.SMTPHost = "smtp.example"
	c.SMTPFrom = "no-reply@example.test"
	if err := c.Validate(); err == nil {
		t.Fatal("production must reject plaintext SMTP")
	}
	c.SMTPTLS = "starttls"
	c.SMTPUsername = "user"
	c.SMTPPassword = "password"
	c.S3Endpoint = "http://localhost:9000"
	if err := c.Validate(); err == nil {
		t.Fatal("production must reject local MinIO endpoint")
	}
	c.S3Endpoint = "https://s3.example"
	if err := c.Validate(); err == nil {
		t.Fatal("production must reject missing METRICS_TOKEN")
	}
	c.MetricsToken = strings.Repeat("x", minMetricsTokenBytes)
	if err := c.Validate(); err != nil {
		t.Fatalf("expected valid production config, got %v", err)
	}
}

func TestValidateProductionRequiresHTTPSStorage(t *testing.T) {
	c := validConfig()
	c.Environment = EnvProduction
	c.PublicURL = "https://app.example.test"
	c.SMTPHost = "smtp.example"
	c.SMTPFrom = "no-reply@example.test"
	c.SMTPTLS = SMTPStartTLS
	c.S3Endpoint = "http://s3.example"
	c.MetricsToken = strings.Repeat("x", minMetricsTokenBytes)
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "S3_ENDPOINT must use HTTPS") {
		t.Fatalf("expected production HTTP storage rejection, got %v", err)
	}
}

func TestValidateRejectsUnknownEnvironment(t *testing.T) {
	c := validConfig()
	c.Environment = "staging"
	if err := c.Validate(); err == nil {
		t.Fatal("unknown APP_ENV must be rejected")
	}
}

func TestLoadNormalizesEnvironment(t *testing.T) {
	t.Setenv("APP_ENV", "  Production ")
	cfg := Load()
	if cfg.Environment != EnvProduction {
		t.Fatalf("expected normalized %q, got %q", EnvProduction, cfg.Environment)
	}
	if !cfg.MetricsAuthRequired() {
		t.Fatal("production environment must require metrics authentication")
	}
}

func TestMetricsAuthRequiredAndIsTestDecisions(t *testing.T) {
	cases := map[string]struct {
		env          string
		authRequired bool
		isTest       bool
	}{
		EnvDevelopment: {env: EnvDevelopment, authRequired: false, isTest: false},
		EnvProduction:  {env: EnvProduction, authRequired: true, isTest: false},
		EnvTest:        {env: EnvTest, authRequired: false, isTest: true},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			c := validConfig()
			c.Environment = want.env
			if got := c.MetricsAuthRequired(); got != want.authRequired {
				t.Fatalf("MetricsAuthRequired() = %v, want %v", got, want.authRequired)
			}
			if got := c.IsTest(); got != want.isTest {
				t.Fatalf("IsTest() = %v, want %v", got, want.isTest)
			}
		})
	}
}

func TestValidateProductionRequiresStrongMetricsToken(t *testing.T) {
	base := validConfig()
	base.Environment = EnvProduction
	base.PublicURL = "https://app.example.test"
	base.SMTPTLS = SMTPStartTLS
	base.SMTPHost = "smtp.example"
	base.SMTPFrom = "no-reply@example.test"
	base.S3Endpoint = "https://s3.example"

	// A token shorter than the minimum is rejected. Load already trims the
	// value before it reaches Validate, so the stored field has no padding.
	base.MetricsToken = strings.Repeat("x", minMetricsTokenBytes-1)
	if err := base.Validate(); err == nil {
		t.Fatal("production must reject METRICS_TOKEN shorter than 32 bytes")
	}

	base.MetricsToken = strings.Repeat("x", minMetricsTokenBytes)
	if err := base.Validate(); err != nil {
		t.Fatalf("expected valid production config with 32-byte token, got %v", err)
	}
}

func TestLoadTrimsMetricsToken(t *testing.T) {
	// Whitespace around the configured token is removed on load so the value
	// used for constant-time comparison matches what a correct client sends.
	trimmed := strings.Repeat("t", minMetricsTokenBytes)
	t.Setenv("METRICS_TOKEN", "  \n"+trimmed+"  ")
	cfg := Load()
	if cfg.MetricsToken != trimmed {
		t.Fatalf("METRICS_TOKEN was not trimmed: %q", cfg.MetricsToken)
	}
}

func TestLoadValidatedAndInvalidEnvironmentValuesUseSafeDefaults(t *testing.T) {
	t.Setenv("DB_MIN_CONNS", "bad")
	t.Setenv("DB_MAX_CONNS", "bad")
	t.Setenv("UPLOAD_MAX_BYTES", "bad")
	t.Setenv("UPLOAD_MAX_PIXELS", "bad")
	t.Setenv("S3_USE_PATH_STYLE", "bad")
	t.Setenv("ACCESS_TOKEN_TTL", "bad")
	t.Setenv("SMTP_DIAL_TIMEOUT", "bad")
	if cfg := Load(); cfg.DatabaseMinConns != 2 || cfg.DatabaseMaxConns != 10 || cfg.UploadMaxBytes <= 0 || cfg.UploadMaxPixels == 0 || !cfg.S3UsePathStyle || cfg.AccessTokenTTL <= 0 || cfg.SMTPDialTimeout <= 0 {
		t.Fatalf("invalid environment values were not replaced safely: %+v", cfg)
	}

	t.Setenv("DATABASE_URL", "postgres://u:p@localhost/db")
	t.Setenv("JWT_SECRET", "a-valid-secret-that-is-at-least-32-bytes-long")
	t.Setenv("SMTP_HOST", "localhost")
	t.Setenv("SMTP_TLS", "off")
	if cfg, err := LoadValidated(); err != nil || cfg == nil {
		t.Fatalf("valid environment was rejected: %v", err)
	}
	t.Setenv("DATABASE_URL", "")
	if cfg, err := LoadValidated(); err == nil || cfg != nil {
		t.Fatal("invalid environment was accepted")
	}
}

func TestValidateReportsBroadConfigurationFailures(t *testing.T) {
	c := validConfig()
	c.Port = "not-a-port"
	c.DatabaseURL = ""
	c.JWTSecret = "short"
	c.AccessTokenTTL = 0
	c.RefreshTokenTTL = 0
	c.VerificationTTL = 0
	c.ResetTTL = 0
	c.PasswordHashCost = 3
	c.DatabaseMinConns = -1
	c.DatabaseMaxConns = 0
	c.AllowedOrigins = []string{"not-an-origin"}
	c.S3Endpoint = "ftp://"
	c.S3Bucket = ""
	c.S3AccessKey = ""
	c.S3SecretKey = ""
	c.UploadMaxBytes = 0
	c.UploadMaxPixels = 0
	c.ChallengeTTL = 0
	c.ViewWindow = 0
	c.PhotoRetention = time.Second
	c.RateLimitRequests = 0
	c.RateLimitWindow = 0
	c.SMTPDialTimeout = 0
	c.SMTPTimeout = 0
	c.SMTPTLS = "ssl"
	c.SMTPPort = 0
	if err := c.Validate(); err == nil {
		t.Fatal("broadly invalid configuration was accepted")
	}
}
