package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

var ErrOIDCUnauthenticated = errors.New("OIDC authentication failed")

// ExternalIdentity is the minimal verified claim set used to attach a
// Keycloak account to a GeoGuessMe user. Raw tokens and unrelated claims never
// leave this boundary.
type ExternalIdentity struct {
	Issuer            string
	Subject           string
	Email             string
	EmailVerified     bool
	PreferredUsername string
	Name              string
}

// IdentityVerifier allows the HTTP transport to accept the production OIDC
// verifier or a deterministic fake in unit tests.
type IdentityVerifier interface {
	VerifyIdentity(context.Context, string) (ExternalIdentity, error)
}

// IdentityAdmin removes the exact upstream identity linked to an application
// user. Implementations must reject issuer mismatches.
type IdentityAdmin interface {
	DeleteIdentity(context.Context, string, string) error
}

// OIDCVerifier validates Keycloak tokens through discovery and its rotating
// JWKS. Issuer, audience, signature, expiry, and the verified-email claim are
// all checked before an identity reaches account-linking code.
type OIDCVerifier struct {
	verifier *oidc.IDTokenVerifier
	issuer   string
	admin    *KeycloakAdmin
}

func NewOIDCVerifier(ctx context.Context, issuerURL, clientID string, clientSecrets ...string) (*OIDCVerifier, error) {
	issuerURL = strings.TrimSpace(issuerURL)
	clientID = strings.TrimSpace(clientID)
	if issuerURL == "" || clientID == "" {
		return nil, errors.New("Keycloak OIDC configuration is incomplete")
	}
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("discover Keycloak OIDC provider: %w", err)
	}
	clientSecret := ""
	if len(clientSecrets) > 0 {
		clientSecret = strings.TrimSpace(clientSecrets[0])
	}
	var admin *KeycloakAdmin
	if clientSecret != "" {
		admin, err = NewKeycloakAdmin(issuerURL, clientID, clientSecret)
		if err != nil {
			return nil, err
		}
	}
	return &OIDCVerifier{
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}),
		issuer:   strings.TrimRight(issuerURL, "/"),
		admin:    admin,
	}, nil
}

func (v *OIDCVerifier) VerifyIdentity(ctx context.Context, authorization string) (ExternalIdentity, error) {
	parts := strings.Fields(strings.TrimSpace(authorization))
	if v == nil || len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ExternalIdentity{}, ErrOIDCUnauthenticated
	}
	verified, err := v.verifier.Verify(ctx, parts[1])
	if err != nil {
		return ExternalIdentity{}, ErrOIDCUnauthenticated
	}
	var claims struct {
		Subject           string `json:"sub"`
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
	}
	if err := verified.Claims(&claims); err != nil {
		return ExternalIdentity{}, ErrOIDCUnauthenticated
	}
	claims.Subject = strings.TrimSpace(claims.Subject)
	claims.Email = strings.TrimSpace(claims.Email)
	if claims.Subject == "" || len(claims.Subject) > 255 || claims.Email == "" || !claims.EmailVerified {
		return ExternalIdentity{}, ErrOIDCUnauthenticated
	}
	return ExternalIdentity{
		Issuer:            verified.Issuer,
		Subject:           claims.Subject,
		Email:             claims.Email,
		EmailVerified:     true,
		PreferredUsername: strings.TrimSpace(claims.PreferredUsername),
		Name:              strings.TrimSpace(claims.Name),
	}, nil
}

// DeleteIdentity obtains a short-lived service-account token and deletes only
// the subject whose issuer exactly matches this verifier. A missing user is an
// idempotent success, which makes retries safe after partial failures.
func (v *OIDCVerifier) DeleteIdentity(ctx context.Context, issuer, subject string) error {
	if v == nil || v.admin == nil {
		return errors.New("Keycloak identity deletion is not configured")
	}
	return v.admin.DeleteIdentity(ctx, issuer, subject)
}
