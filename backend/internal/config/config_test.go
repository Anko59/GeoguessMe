package config

import (
	"os"
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
	c.SMTPTLS = "off"
	c.SMTPHost = "smtp.example"
	c.SMTPFrom = "no-reply@example.test"
	if err := c.Validate(); err == nil {
		t.Fatal("production must reject plaintext SMTP")
	}
	c.SMTPTLS = "starttls"
	c.SMTPUsername = "user"
	c.S3Endpoint = "http://localhost:9000"
	if err := c.Validate(); err == nil {
		t.Fatal("production must reject local MinIO endpoint")
	}
	c.S3Endpoint = "https://s3.example"
	if err := c.Validate(); err != nil {
		t.Fatalf("expected valid production config, got %v", err)
	}
}
