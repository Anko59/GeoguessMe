package config

import (
	"strings"
	"testing"
)

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
