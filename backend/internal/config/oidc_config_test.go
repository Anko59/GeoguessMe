package config

import (
	"strings"
	"testing"
)

func TestValidateOIDCConfiguration(t *testing.T) {
	c := validConfig()
	c.OIDCEnabled = true
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "OIDC_ISSUER_URL") || !strings.Contains(err.Error(), "OIDC_CLIENT_ID") || !strings.Contains(err.Error(), "OIDC_CLIENT_SECRET") {
		t.Fatalf("expected incomplete OIDC configuration rejection, got %v", err)
	}
	c.OIDCIssuerURL = "http://keycloak:8080/realms/geoguessme"
	c.OIDCClientID = "geoguessme-web"
	c.OIDCClientSecret = "test-client-secret"
	if err := c.Validate(); err != nil {
		t.Fatalf("expected valid development OIDC configuration, got %v", err)
	}
	c.Environment = EnvProduction
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "OIDC_ISSUER_URL must use HTTPS") {
		t.Fatalf("expected production HTTPS OIDC rejection, got %v", err)
	}
}

func TestValidateOIDCSocialProviders(t *testing.T) {
	c := validConfig()
	c.OIDCSocialProviders = []string{"google"}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected supported social providers, got %v", err)
	}

	c.OIDCSocialProviders = []string{"google", "google", "apple", "github"}
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "duplicate provider") || !strings.Contains(err.Error(), "unsupported provider") {
		t.Fatalf("expected invalid social provider rejection, got %v", err)
	}
}

func TestLoadOIDCSocialProviders(t *testing.T) {
	unsetAllConfigVariables()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("default OIDC provider configuration must load, got %v", err)
	}
	if len(cfg.OIDCSocialProviders) != 0 {
		t.Fatalf("expected no social providers by default, got %v", cfg.OIDCSocialProviders)
	}

	t.Setenv("OIDC_SOCIAL_PROVIDERS", " google ")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("configured OIDC providers must load, got %v", err)
	}
	if got := strings.Join(cfg.OIDCSocialProviders, ","); got != "google" {
		t.Fatalf("expected normalized social provider list, got %q", got)
	}
}
