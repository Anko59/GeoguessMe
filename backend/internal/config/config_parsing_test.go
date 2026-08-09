package config

import (
	"os"
	"strings"
	"testing"
)

// Strict-parsing tests: defaults apply only to absent variables, and a
// variable that is present but malformed fails loading with an aggregated
// error that names every offending variable.

func TestLoadRejectsMalformedPresentValuesWithVariableNames(t *testing.T) {
	t.Setenv("DB_MIN_CONNS", "bad")
	t.Setenv("DB_MAX_CONNS", "bad")
	t.Setenv("UPLOAD_MAX_BYTES", "bad")
	t.Setenv("AVATAR_MAX_BYTES", "bad")
	t.Setenv("UPLOAD_MAX_PIXELS", "bad")
	t.Setenv("S3_USE_PATH_STYLE", "bad")
	t.Setenv("ACCESS_TOKEN_TTL", "bad")
	t.Setenv("SMTP_DIAL_TIMEOUT", "bad")
	t.Setenv("BCRYPT_COST", "many")

	cfg, err := Load()
	if err == nil {
		t.Fatal("malformed present values must fail loading")
	}
	for _, name := range []string{"DB_MIN_CONNS", "DB_MAX_CONNS", "UPLOAD_MAX_BYTES", "AVATAR_MAX_BYTES", "UPLOAD_MAX_PIXELS", "S3_USE_PATH_STYLE", "ACCESS_TOKEN_TTL", "SMTP_DIAL_TIMEOUT", "BCRYPT_COST"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("load error must name %s, got: %v", name, err)
		}
	}
	// The config is still returned so callers can aggregate parse and
	// validation errors, but the malformed fields hold their defaults.
	if cfg == nil {
		t.Fatal("Load must return a config alongside the aggregated error")
	}
	if cfg.Port != "8080" {
		t.Errorf("malformed PORT should hold the default alongside the error, got %q", cfg.Port)
	}
}

func TestLoadRejectsMalformedCIDRAndURL(t *testing.T) {
	t.Setenv("TRUSTED_PROXY_CIDRS", "172.16.0.0/12,not-a-cidr")
	t.Setenv("PUBLIC_URL", "ht!tp://bad url")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TRUSTED_PROXY_CIDRS") {
		t.Fatalf("malformed CIDR must fail load with the variable name, got %v", err)
	}

	t.Setenv("TRUSTED_PROXY_CIDRS", "172.16.0.0/12")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "PUBLIC_URL") {
		t.Fatalf("malformed URL must fail load with the variable name, got %v", err)
	}

	t.Setenv("PUBLIC_URL", "https://example.test")
	if _, err := Load(); err != nil {
		t.Fatalf("valid CIDR and URL must load, got %v", err)
	}
}

func TestLoadTreatsEmptyStringVariablesAsUnset(t *testing.T) {
	// Optional features are disabled with an empty assignment; that is a valid
	// state, not a parse error.
	t.Setenv("STORAGE_DRIVER", "")
	t.Setenv("TRUSTED_PROXY_CIDRS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("empty optional assignments must load, got %v", err)
	}
	if cfg.StorageDriver != "" {
		t.Errorf("empty STORAGE_DRIVER should stay empty, got %q", cfg.StorageDriver)
	}
	if len(cfg.TrustedProxyCIDRs) != 0 {
		t.Errorf("empty TRUSTED_PROXY_CIDRS should yield no trusted proxies, got %v", cfg.TrustedProxyCIDRs)
	}
}

func TestLoadPreservesExplicitEmptyDefaultedStrings(t *testing.T) {
	t.Setenv("S3_ACCESS_KEY", "")
	t.Setenv("S3_SECRET_KEY", "")
	t.Setenv("ALLOWED_ORIGINS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("empty strings are semantic rather than parse errors, got %v", err)
	}
	if cfg.S3AccessKey != "" || cfg.S3SecretKey != "" {
		t.Fatalf("explicit empty credentials were replaced by defaults: %q / %q", cfg.S3AccessKey, cfg.S3SecretKey)
	}
	if len(cfg.AllowedOrigins) != 0 {
		t.Fatalf("explicit empty origins were replaced by defaults: %v", cfg.AllowedOrigins)
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("explicit empty required settings must fail semantic validation")
	}
}

func TestLoadRejectsOutOfRangeDatabaseConnectionCounts(t *testing.T) {
	t.Setenv("DB_MIN_CONNS", "4294967297")
	t.Setenv("DB_MAX_CONNS", "9223372036854775807")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DB_MIN_CONNS") || !strings.Contains(err.Error(), "DB_MAX_CONNS") {
		t.Fatalf("out-of-range database counts must fail before int32 conversion, got %v", err)
	}
}

func TestValidateRejectsUnknownStorageDriver(t *testing.T) {
	cfg := validConfig()
	cfg.StorageDriver = "locla"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "STORAGE_DRIVER") {
		t.Fatalf("unknown storage driver must be rejected, got %v", err)
	}
}

func TestLoadParsesStorageDriver(t *testing.T) {
	t.Setenv("STORAGE_DRIVER", "local")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("STORAGE_DRIVER=local must load, got %v", err)
	}
	if cfg.StorageDriver != "local" {
		t.Errorf("expected StorageDriver local, got %q", cfg.StorageDriver)
	}
}

func TestLoadValidatedCombinesParseAndValidationErrors(t *testing.T) {
	// A malformed typed value and a semantic problem surface together.
	t.Setenv("PORT", "not-a-port")
	t.Setenv("DB_MAX_CONNS", "2")
	t.Setenv("DB_MIN_CONNS", "50")
	t.Setenv("DATABASE_URL", "postgres://u:p@localhost/db")
	t.Setenv("JWT_SECRET", "a-valid-secret-that-is-at-least-32-bytes-long")
	t.Setenv("SMTP_HOST", "localhost")
	t.Setenv("SMTP_TLS", "off")
	if cfg, err := LoadValidated(); err == nil || cfg == nil {
		t.Fatal("LoadValidated must return an aggregated error")
	} else if !strings.Contains(err.Error(), "PORT") {
		t.Errorf("LoadValidated error must include the parse failure, got %v", err)
	} else if !strings.Contains(err.Error(), "DB_MAX_CONNS") {
		t.Errorf("LoadValidated error must include semantic failures, got %v", err)
	}

	// With parse errors fixed, semantic validation still applies.
	t.Setenv("PORT", "8080")
	if _, err := LoadValidated(); err == nil {
		t.Fatal("DB_MAX_CONNS below DB_MIN_CONNS must be rejected")
	}
}

func TestEnvironmentTemplatesParseCleanlyUnderStrictLoader(t *testing.T) {
	// Every checked-in environment template must satisfy strict parsing so the
	// documented deployment paths never hit a load error. Template values are
	// placeholders, so only parse-level strictness is asserted; semantic
	// production validation is exercised by the deployment rehearsals.
	templates := []string{
		"development.env.example",
		"test.env.example",
		"dev.env.example",
		"production.env.example",
	}
	for _, name := range templates {
		t.Run(name, func(t *testing.T) {
			path := "../../../deployment/env/" + name
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read template: %v", err)
			}
			unsetAllConfigVariables()
			for _, line := range strings.Split(string(content), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				key, value, found := strings.Cut(line, "=")
				if !found {
					t.Fatalf("unparsable line %q", line)
				}
				t.Setenv(strings.TrimSpace(key), value)
			}
			if _, err := Load(); err != nil {
				t.Fatalf("template %s failed strict parsing: %v", name, err)
			}
		})
	}
}
