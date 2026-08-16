package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const legacyPasswordActionLifespan = 24 * time.Hour

// KeycloakAdmin is the narrow service-account client used for account
// lifecycle operations. It never accepts raw admin credentials: the
// application client receives only the realm-management roles it needs.
type KeycloakAdmin struct {
	issuer       string
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

// LegacyProvisionResult describes an idempotent legacy-user provisioning
// attempt without exposing the player's email or Keycloak subject in logs.
type LegacyProvisionResult struct {
	Created         bool
	ActionEmailSent bool
}

type keycloakUser struct {
	ID              string   `json:"id"`
	Email           string   `json:"email"`
	EmailVerified   bool     `json:"emailVerified"`
	RequiredActions []string `json:"requiredActions"`
}

func NewKeycloakAdmin(issuerURL, clientID, clientSecret string) (*KeycloakAdmin, error) {
	admin := &KeycloakAdmin{
		issuer: strings.TrimRight(strings.TrimSpace(issuerURL), "/"), clientID: strings.TrimSpace(clientID),
		clientSecret: strings.TrimSpace(clientSecret), httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	if admin.issuer == "" || admin.clientID == "" || admin.clientSecret == "" {
		return nil, errors.New("Keycloak administration configuration is incomplete")
	}
	if _, err := keycloakAdminUsersURL(admin.issuer); err != nil {
		return nil, err
	}
	return admin, nil
}

// DeleteIdentity deletes only a subject from this client's configured realm.
// A missing user is an idempotent success, which makes deletion retries safe.
func (a *KeycloakAdmin) DeleteIdentity(ctx context.Context, issuer, subject string) error {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	subject = strings.TrimSpace(subject)
	if a == nil || issuer == "" || issuer != a.issuer || subject == "" {
		return errors.New("Keycloak identity deletion is not configured")
	}
	token, err := a.serviceAccountToken(ctx)
	if err != nil {
		return err
	}
	deleteURL, err := keycloakAdminUserURL(a.issuer, subject)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := a.client().Do(req)
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

// ProvisionLegacyUser creates a passwordless Keycloak user for an address the
// application already verified, then sends Keycloak's UPDATE_PASSWORD action.
// Replays reuse the exact verified Keycloak user and resend only while that
// required action remains outstanding.
func (a *KeycloakAdmin) ProvisionLegacyUser(ctx context.Context, email, redirectURI string) (LegacyProvisionResult, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if a == nil || email == "" {
		return LegacyProvisionResult{}, errors.New("legacy Keycloak provisioning is not configured")
	}
	token, err := a.serviceAccountToken(ctx)
	if err != nil {
		return LegacyProvisionResult{}, err
	}
	user, err := a.findUserByEmail(ctx, token, email)
	if err != nil {
		return LegacyProvisionResult{}, err
	}
	result := LegacyProvisionResult{}
	if user == nil {
		user, err = a.createLegacyUser(ctx, token, email)
		if err != nil {
			return result, err
		}
		result.Created = true
	}
	if user == nil {
		return result, errors.New("created Keycloak user could not be resolved")
	}
	if !user.EmailVerified || !strings.EqualFold(strings.TrimSpace(user.Email), email) {
		return result, errors.New("matching Keycloak user does not have the same verified email")
	}
	if result.Created || containsString(user.RequiredActions, "UPDATE_PASSWORD") {
		if err := a.sendRequiredActionsEmail(ctx, token, user.ID, redirectURI); err != nil {
			return result, err
		}
		result.ActionEmailSent = true
	}
	return result, nil
}

func (a *KeycloakAdmin) findUserByEmail(ctx context.Context, token, email string) (*keycloakUser, error) {
	usersURL, err := keycloakAdminUsersURL(a.issuer)
	if err != nil {
		return nil, err
	}
	parsed, _ := url.Parse(usersURL)
	query := parsed.Query()
	query.Set("email", email)
	query.Set("exact", "true")
	parsed.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := a.client().Do(req)
	if err != nil {
		// net/http request errors include the full query URL, which contains the
		// player's email. Keep operator logs aggregate-only.
		return nil, errors.New("find Keycloak user request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("find Keycloak user: unexpected HTTP status %d", response.StatusCode)
	}
	var users []keycloakUser
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&users); err != nil {
		return nil, fmt.Errorf("find Keycloak user: invalid response: %w", err)
	}
	var match *keycloakUser
	for index := range users {
		if !strings.EqualFold(strings.TrimSpace(users[index].Email), email) {
			continue
		}
		if match != nil {
			return nil, errors.New("multiple Keycloak users have the same email")
		}
		candidate := users[index]
		match = &candidate
	}
	return match, nil
}

func (a *KeycloakAdmin) createLegacyUser(ctx context.Context, token, email string) (*keycloakUser, error) {
	payload, err := json.Marshal(map[string]any{
		"enabled": true, "username": email, "email": email, "emailVerified": true,
		"requiredActions": []string{"UPDATE_PASSWORD"},
	})
	if err != nil {
		return nil, err
	}
	usersURL, err := keycloakAdminUsersURL(a.issuer)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, usersURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response, err := a.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("create Keycloak user: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create Keycloak user: unexpected HTTP status %d", response.StatusCode)
	}
	location, err := url.Parse(response.Header.Get("Location"))
	if err != nil || strings.Trim(location.Path, "/") == "" {
		return a.findUserByEmail(ctx, token, email)
	}
	parts := strings.Split(strings.Trim(location.Path, "/"), "/")
	subject, err := url.PathUnescape(parts[len(parts)-1])
	if err != nil || strings.TrimSpace(subject) == "" {
		return a.findUserByEmail(ctx, token, email)
	}
	return &keycloakUser{ID: subject, Email: email, EmailVerified: true, RequiredActions: []string{"UPDATE_PASSWORD"}}, nil
}

func (a *KeycloakAdmin) sendRequiredActionsEmail(ctx context.Context, token, subject, redirectURI string) error {
	userURL, err := keycloakAdminUserURL(a.issuer, subject)
	if err != nil {
		return err
	}
	actions, _ := json.Marshal([]string{"UPDATE_PASSWORD"})
	endpoint, _ := url.Parse(userURL + "/execute-actions-email")
	query := endpoint.Query()
	query.Set("lifespan", fmt.Sprintf("%.0f", legacyPasswordActionLifespan.Seconds()))
	if redirectURI = strings.TrimSpace(redirectURI); redirectURI != "" {
		query.Set("client_id", a.clientID)
		query.Set("redirect_uri", redirectURI)
	}
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint.String(), bytes.NewReader(actions))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response, err := a.client().Do(req)
	if err != nil {
		// net/http errors include the full user endpoint, including the opaque
		// Keycloak subject. Migration logs intentionally stay aggregate-only.
		return errors.New("send Keycloak password setup email request failed")
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("send Keycloak password setup email: unexpected HTTP status %d", response.StatusCode)
	}
	return nil
}

func (a *KeycloakAdmin) serviceAccountToken(ctx context.Context) (string, error) {
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {a.clientID}, "client_secret": {a.clientSecret}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.issuer+"/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := a.client().Do(req)
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

func (a *KeycloakAdmin) client() *http.Client {
	if a.httpClient != nil {
		return a.httpClient
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func keycloakAdminUsersURL(issuer string) (string, error) {
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
	parsed.Path = prefix + "/admin/realms/" + url.PathEscape(realm) + "/users"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func keycloakAdminUserURL(issuer, subject string) (string, error) {
	usersURL, err := keycloakAdminUsersURL(issuer)
	if err != nil {
		return "", err
	}
	return usersURL + "/" + url.PathEscape(subject), nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
