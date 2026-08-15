package config

import (
	"strings"
	"testing"
)

func TestValidateOIDCConfiguration(t *testing.T) {
	c := validConfig()
	c.OIDCEnabled = true
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "OIDC_ISSUER_URL") || !strings.Contains(err.Error(), "OIDC_CLIENT_ID") {
		t.Fatalf("expected incomplete OIDC configuration rejection, got %v", err)
	}
	c.OIDCIssuerURL = "http://keycloak:8080/realms/geoguessme"
	c.OIDCClientID = "geoguessme-web"
	if err := c.Validate(); err != nil {
		t.Fatalf("expected valid development OIDC configuration, got %v", err)
	}
	c.Environment = EnvProduction
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "OIDC_ISSUER_URL must use HTTPS") {
		t.Fatalf("expected production HTTPS OIDC rejection, got %v", err)
	}
}
