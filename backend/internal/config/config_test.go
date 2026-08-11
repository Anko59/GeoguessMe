package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

// allConfigVariables lists every environment variable the strict loader reads.
// Tests use it to guarantee a hermetic environment when asserting defaults.
var allConfigVariables = []string{
	"APP_ENV", "PORT", "PUBLIC_URL", "STORAGE_DRIVER",
	"DATABASE_URL", "DB_MIN_CONNS", "DB_MAX_CONNS", "JWT_SECRET",
	"ACCESS_TOKEN_TTL", "REFRESH_TOKEN_TTL", "VERIFICATION_TOKEN_TTL", "RESET_TOKEN_TTL", "BCRYPT_COST",
	"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_TLS", "SMTP_DIAL_TIMEOUT", "SMTP_TIMEOUT",
	"S3_ENDPOINT", "S3_REGION", "S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY", "S3_USE_PATH_STYLE",
	"ALLOWED_ORIGINS", "TRUSTED_PROXY_CIDRS",
	"UPLOAD_MAX_BYTES", "AVATAR_MAX_BYTES", "UPLOAD_MAX_PIXELS",
	"CHALLENGE_TTL", "LOCATION_HIDE_DURATION", "PHOTO_VIEW_WINDOW", "PHOTO_RETENTION", "UPLOAD_DIR",
	"RATE_LIMIT_REQUESTS", "RATE_LIMIT_WINDOW", "LOG_LEVEL", "METRICS_TOKEN",
	"RATE_LIMIT_LOGIN", "RATE_LIMIT_SIGNUP", "RATE_LIMIT_EMAIL", "RATE_LIMIT_RESET", "RATE_LIMIT_PUSH", "RATE_LIMIT_DEFAULT", "RATE_LIMIT_FAIL_CLOSED", "RATE_LIMIT_STORE_CAP",
	"VAPID_PUBLIC_KEY", "VAPID_PRIVATE_KEY", "VAPID_SUBJECT", "PUSH_ENDPOINT_ALLOWLIST",
	"PUSH_MAX_SUBSCRIPTIONS_PER_USER", "PUSH_SUBSCRIPTION_EXPIRY",
	"PUSH_DELIVERY_WORKERS", "PUSH_DELIVERY_PER_HOST", "PUSH_DELIVERY_TIMEOUT", "PUSH_QUEUE_DEPTH",
}

func unsetAllConfigVariables() {
	for _, key := range allConfigVariables {
		os.Unsetenv(key)
	}
}

func TestLoadDefaults(t *testing.T) {
	// Clear every configuration variable so defaults are used.
	unsetAllConfigVariables()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("absent variables must not produce load errors, got %v", err)
	}

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
	if cfg.AvatarMaxBytes != 25*1024*1024 {
		t.Errorf("Expected default AvatarMaxBytes 25 MiB, got %d", cfg.AvatarMaxBytes)
	}
	if len(cfg.AllowedOrigins) != 2 {
		t.Errorf("Expected 2 default AllowedOrigins, got %d", len(cfg.AllowedOrigins))
	}
	if len(cfg.PushEndpointAllowlist) != 4 || cfg.PushEndpointAllowlist[0] != "fcm.googleapis.com" {
		t.Errorf("Expected 4 default PushEndpointAllowlist entries starting with fcm.googleapis.com, got %v", cfg.PushEndpointAllowlist)
	}
	if cfg.PushMaxSubscriptionsPerUser != 5 {
		t.Errorf("Expected default PushMaxSubscriptionsPerUser 5, got %d", cfg.PushMaxSubscriptionsPerUser)
	}
	if cfg.PushSubscriptionExpiry != 90*24*time.Hour {
		t.Errorf("Expected default PushSubscriptionExpiry 90d, got %v", cfg.PushSubscriptionExpiry)
	}
	if cfg.PushDeliveryWorkers != 4 || cfg.PushDeliveryPerHost != 2 || cfg.PushDeliveryTimeout != 5*time.Second || cfg.PushQueueDepth != 256 {
		t.Errorf("unexpected default push delivery bounds: workers=%d perHost=%d timeout=%v queue=%d",
			cfg.PushDeliveryWorkers, cfg.PushDeliveryPerHost, cfg.PushDeliveryTimeout, cfg.PushQueueDepth)
	}
	if cfg.StorageDriver != "" {
		t.Errorf("Expected default StorageDriver empty (S3), got %q", cfg.StorageDriver)
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
	os.Setenv("PUSH_MAX_SUBSCRIPTIONS_PER_USER", "8")
	os.Setenv("PUSH_DELIVERY_TIMEOUT", "3s")

	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("DATABASE_URL")
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("ALLOWED_ORIGINS")
		os.Unsetenv("RATE_LIMIT_REQUESTS")
		os.Unsetenv("RATE_LIMIT_WINDOW")
		os.Unsetenv("UPLOAD_DIR")
		os.Unsetenv("PUSH_MAX_SUBSCRIPTIONS_PER_USER")
		os.Unsetenv("PUSH_DELIVERY_TIMEOUT")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("valid overrides must load, got %v", err)
	}

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
	if cfg.PushMaxSubscriptionsPerUser != 8 {
		t.Errorf("Expected PushMaxSubscriptionsPerUser 8, got %d", cfg.PushMaxSubscriptionsPerUser)
	}
	if cfg.PushDeliveryTimeout != 3*time.Second {
		t.Errorf("Expected PushDeliveryTimeout 3s, got %v", cfg.PushDeliveryTimeout)
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
		AvatarMaxBytes:    25 * 1024 * 1024,
		UploadMaxPixels:   25_000_000,
		ChallengeTTL:      24 * time.Hour,
		ViewWindow:        10 * time.Second,
		LocationHide:      48 * time.Hour,
		PhotoRetention:    30 * 24 * time.Hour,
		RateLimitRequests: 10,
		RateLimitWindow:   time.Minute,
		RateLimitPolicies: []RateLimitPolicy{
			{Name: "login", Buckets: []RateLimitBucket{
				{Type: "identity", Limit: 10, Window: time.Minute},
				{Type: "trustedIP", Limit: 30, Window: time.Minute},
				{Type: "global", Limit: 300, Window: time.Minute},
			}},
			{Name: "signup", Buckets: []RateLimitBucket{
				{Type: "identity", Limit: 3, Window: time.Hour},
				{Type: "trustedIP", Limit: 5, Window: time.Hour},
				{Type: "global", Limit: 60, Window: time.Minute},
			}},
			{Name: "email", Buckets: []RateLimitBucket{
				{Type: "identity", Limit: 3, Window: time.Hour},
				{Type: "trustedIP", Limit: 5, Window: time.Hour},
				{Type: "global", Limit: 30, Window: time.Minute},
			}},
			{Name: "reset", Buckets: []RateLimitBucket{
				{Type: "trustedIP", Limit: 10, Window: time.Hour},
			}},
			{Name: "push", Buckets: []RateLimitBucket{
				{Type: "user", Limit: 10, Window: time.Hour},
				{Type: "trustedIP", Limit: 20, Window: time.Hour},
			}},
			{Name: "default", Buckets: []RateLimitBucket{
				{Type: "identity", Limit: 10, Window: time.Minute},
				{Type: "trustedIP", Limit: 60, Window: time.Minute},
			}},
		},
		RateLimitFailClosed: []string{"login", "signup", "email", "reset"},
		RateLimitStoreCap:   50_000,

		PushMaxSubscriptionsPerUser: 5,
		PushSubscriptionExpiry:      90 * 24 * time.Hour,
		PushDeliveryWorkers:         4,
		PushDeliveryPerHost:         2,
		PushDeliveryTimeout:         5 * time.Second,
		PushQueueDepth:              256,
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
		"zero rate store cap":        func(c *Config) { c.RateLimitStoreCap = 0 },
		"zero push sub cap":          func(c *Config) { c.PushMaxSubscriptionsPerUser = 0 },
		"zero push expiry":           func(c *Config) { c.PushSubscriptionExpiry = 0 },
		"zero push workers":          func(c *Config) { c.PushDeliveryWorkers = 0 },
		"zero push per-host":         func(c *Config) { c.PushDeliveryPerHost = 0 },
		"zero push timeout":          func(c *Config) { c.PushDeliveryTimeout = 0 },
		"zero push queue depth":      func(c *Config) { c.PushQueueDepth = 0 },
		"zero policy limit":          func(c *Config) { c.RateLimitPolicies[0].Buckets[0].Limit = 0 },
		"zero policy window":         func(c *Config) { c.RateLimitPolicies[0].Buckets[0].Window = 0 },
		"unknown bucket type":        func(c *Config) { c.RateLimitPolicies[0].Buckets[0].Type = "host" },
		"duplicate bucket type": func(c *Config) {
			c.RateLimitPolicies[0].Buckets = append(c.RateLimitPolicies[0].Buckets, c.RateLimitPolicies[0].Buckets[0])
		},
		"fail-closed unknown policy": func(c *Config) { c.RateLimitFailClosed = []string{"admin"} },
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
	c.VapidPublicKey = "example-public-key"
	c.VapidPrivateKey = "example-private-key"
	c.VapidSubject = "mailto:ops@example.com"
	c.PushEndpointAllowlist = []string{"fcm.googleapis.com"}
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
	cfg, err := Load()
	if err != nil {
		t.Fatalf("valid APP_ENV must load, got %v", err)
	}
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
	// Production also requires a stable VAPID keypair and contact subject so
	// browser push subscriptions survive restarts.
	base.VapidPublicKey = "example-public-key"
	base.VapidPrivateKey = "example-private-key"
	base.VapidSubject = "mailto:ops@example.com"
	base.PushEndpointAllowlist = []string{"fcm.googleapis.com"}
	if err := base.Validate(); err != nil {
		t.Fatalf("expected valid production config with 32-byte token, got %v", err)
	}
}

func TestValidateVapidRules(t *testing.T) {
	// Outside production, VAPID keys are optional (ephemeral keys are minted at
	// startup), but a half-set pair is always wrong.
	halfSet := validConfig()
	halfSet.VapidPublicKey = "only-public"
	if err := halfSet.Validate(); err == nil || !strings.Contains(err.Error(), "VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY must be provided together") {
		t.Fatalf("expected partial VAPID key rejection, got %v", err)
	}

	// A subject is rejected unless a complete keypair enables push.
	badSubject := validConfig()
	badSubject.VapidSubject = "ftp://not-valid"
	if err := badSubject.Validate(); err == nil || !strings.Contains(err.Error(), "VAPID_SUBJECT requires") {
		t.Fatalf("expected VAPID subject without keys rejection, got %v", err)
	}

	// Production without VAPID keys is valid: push is explicitly disabled.
	prod := validConfig()
	prod.Environment = EnvProduction
	prod.PublicURL = "https://app.example.test"
	prod.SMTPTLS = SMTPStartTLS
	prod.SMTPHost = "smtp.example"
	prod.SMTPFrom = "no-reply@example.test"
	prod.S3Endpoint = "https://s3.example"
	prod.MetricsToken = strings.Repeat("x", minMetricsTokenBytes)
	if err := prod.Validate(); err != nil {
		t.Fatalf("expected valid production config without VAPID, got %v", err)
	}
	// A complete keypair requires a syntactically valid RFC 8292 contact.
	prod.VapidPublicKey = "example-public-key"
	prod.VapidPrivateKey = "example-private-key"
	if err := prod.Validate(); err == nil || !strings.Contains(err.Error(), "VAPID_SUBJECT must be") {
		t.Fatalf("expected missing VAPID subject rejection, got %v", err)
	}
	prod.VapidSubject = "mailto:"
	if err := prod.Validate(); err == nil || !strings.Contains(err.Error(), "VAPID_SUBJECT must be") {
		t.Fatalf("expected malformed VAPID subject rejection, got %v", err)
	}
	prod.VapidSubject = "https://example.test/contact"
	prod.PushEndpointAllowlist = []string{"fcm.googleapis.com", "wns.windows.com"}
	if err := prod.Validate(); err != nil {
		t.Fatalf("expected valid production config, got %v", err)
	}
}

func TestValidateProductionRejectsEmptyPushAllowlist(t *testing.T) {
	c := validConfig()
	c.Environment = EnvProduction
	c.PublicURL = "https://app.example.test"
	c.SMTPTLS = SMTPStartTLS
	c.SMTPHost = "smtp.example"
	c.SMTPFrom = "no-reply@example.test"
	c.S3Endpoint = "https://s3.example"
	c.MetricsToken = strings.Repeat("x", minMetricsTokenBytes)
	c.VapidPublicKey = "example-public-key"
	c.VapidPrivateKey = "example-private-key"
	c.VapidSubject = "mailto:ops@example.com"
	// Push enabled with an empty allowlist must reject production startup.
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "PUSH_ENDPOINT_ALLOWLIST") {
		t.Fatalf("expected empty push allowlist rejection, got %v", err)
	}
	c.PushEndpointAllowlist = []string{"fcm.googleapis.com"}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected valid production config with allowlist, got %v", err)
	}
}

func TestValidateRejectsMalformedPushAllowlist(t *testing.T) {
	c := validConfig()
	c.PushEndpointAllowlist = []string{
		"https://fcm.googleapis.com", "", "bad host", ".example.com",
		"example.com.", "two..dots.example", "-prefix.example", "suffix-.example",
	}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "allowlist domain") {
		t.Fatalf("expected malformed allowlist rejection, got %v", err)
	}
}

func TestLoadTrimsMetricsToken(t *testing.T) {
	// Whitespace around the configured token is removed on load so the value
	// used for constant-time comparison matches what a correct client sends.
	trimmed := strings.Repeat("t", minMetricsTokenBytes)
	t.Setenv("METRICS_TOKEN", "  \n"+trimmed+"  ")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("valid METRICS_TOKEN must load, got %v", err)
	}
	if cfg.MetricsToken != trimmed {
		t.Fatalf("METRICS_TOKEN was not trimmed: %q", cfg.MetricsToken)
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
	c.AvatarMaxBytes = 0
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
