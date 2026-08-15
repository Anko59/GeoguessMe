package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

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
	verifier     *oidc.IDTokenVerifier
	issuer       string
	clientID     string
	clientSecret string
	httpClient   *http.Client
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
	return &OIDCVerifier{
		verifier: provider.Verifier(&oidc.Config{ClientID: clientID}), issuer: strings.TrimRight(issuerURL, "/"),
		clientID: clientID, clientSecret: clientSecret, httpClient: &http.Client{Timeout: 10 * time.Second},
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
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	subject = strings.TrimSpace(subject)
	if v == nil || issuer == "" || issuer != v.issuer || subject == "" || v.clientSecret == "" {
		return errors.New("Keycloak identity deletion is not configured")
	}
	token, err := v.serviceAccountToken(ctx)
	if err != nil {
		return err
	}
	deleteURL, err := keycloakAdminUserURL(v.issuer, subject)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := v.client().Do(req)
	if err != nil {
		return fmt.Errorf("delete Keycloak identity: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusNoContent && response.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete Keycloak identity: unexpected HTTP status %d", response.StatusCode)
	}
	return nil
}

func (v *OIDCVerifier) serviceAccountToken(ctx context.Context) (string, error) {
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {v.clientID}, "client_secret": {v.clientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.issuer+"/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := v.client().Do(req)
	if err != nil {
		return "", fmt.Errorf("request Keycloak service token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return "", fmt.Errorf("request Keycloak service token: unexpected HTTP status %d", response.StatusCode)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&payload); err != nil || strings.TrimSpace(payload.AccessToken) == "" {
		return "", errors.New("request Keycloak service token: invalid response")
	}
	return payload.AccessToken, nil
}

func (v *OIDCVerifier) client() *http.Client {
	if v.httpClient != nil {
		return v.httpClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func keycloakAdminUserURL(issuer, subject string) (string, error) {
	parsed, err := url.Parse(issuer)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid Keycloak issuer URL")
	}
	marker := strings.LastIndex(parsed.Path, "/realms/")
	if marker < 0 || strings.Trim(parsed.Path[marker+len("/realms/"):], "/") == "" {
		return "", errors.New("Keycloak issuer URL does not contain a realm")
	}
	prefix := strings.TrimSuffix(parsed.Path[:marker], "/")
	realm := strings.Trim(parsed.Path[marker+len("/realms/"):], "/")
	parsed.Path = prefix + "/admin/realms/" + url.PathEscape(realm) + "/users/" + url.PathEscape(subject)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
