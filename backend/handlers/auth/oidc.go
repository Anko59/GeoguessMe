package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"geoguessme/handlers"
	authsvc "geoguessme/internal/auth"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	"geoguessme/internal/validation"

	"github.com/google/uuid"
)

const (
	oidcLinkCookie = "oidc_link_intent"
	oidcLinkTTL    = 10 * time.Minute
)

// OIDCConfig reports runtime capability so one immutable frontend image can
// hide social-login controls when the identity stack is intentionally absent.
func (a *AuthAPI) OIDCConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		handlers.MethodNotAllowed(w)
		return
	}
	handlers.WriteJSON(w, http.StatusOK, map[string]any{
		"enabled":    a.cfg.OIDCEnabled,
		"login_path": "/oauth2/start",
	})
}

// StartOIDCLink binds a short-lived opaque cookie to the already-authenticated
// application user before the browser leaves for Keycloak.
func (a *AuthAPI) StartOIDCLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	if !a.cfg.OIDCEnabled || a.oidc == nil {
		handlers.WriteError(w, http.StatusServiceUnavailable, "oidc_unavailable", "Social login is unavailable")
		return
	}
	raw, err := authsvc.GenerateOpaqueToken(32)
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to start account linking")
		return
	}
	expiresAt := time.Now().Add(oidcLinkTTL)
	if err := a.repos.CreateOIDCLinkIntent(r.Context(), handlers.GetUserIDFromContext(r), authsvc.HashToken(raw), expiresAt); err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to start account linking")
		return
	}
	a.setOIDCLinkCookie(w, raw, expiresAt)
	w.WriteHeader(http.StatusNoContent)
}

// ExchangeOIDCSession verifies the Keycloak token forwarded only by OAuth2
// Proxy, resolves or links the canonical application user, then issues the
// existing short-lived GeoGuessMe access/refresh session. Keycloak tokens are
// never returned to browser JavaScript.
func (a *AuthAPI) ExchangeOIDCSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	if !a.cfg.OIDCEnabled || a.oidc == nil {
		handlers.WriteError(w, http.StatusServiceUnavailable, "oidc_unavailable", "Social login is unavailable")
		return
	}
	identity, err := a.oidc.VerifyIdentity(r.Context(), r.Header.Get("Authorization"))
	if err != nil || validation.ValidateEmail(identity.Email) != nil || !identity.EmailVerified {
		handlers.WriteError(w, http.StatusUnauthorized, "oidc_authentication_failed", "Social login could not be verified")
		return
	}
	persisted := repository.OIDCIdentity{Issuer: identity.Issuer, Subject: identity.Subject, Email: identity.Email}
	now := time.Now()
	var userErr error
	var userID = uuid.NewString()
	var user *models.User
	if cookie, cookieErr := r.Cookie(oidcLinkCookie); cookieErr == nil && cookie.Value != "" {
		a.clearOIDCLinkCookie(w)
		user, userErr = a.repos.LinkOIDCIdentity(r.Context(), authsvc.HashToken(cookie.Value), persisted, now)
	} else {
		user, userErr = a.repos.ResolveOIDCIdentity(r.Context(), persisted, userID, oidcUsername(identity), a.randomAvatar(), now)
	}
	switch {
	case errors.Is(userErr, repository.ErrOIDCAccountLinkRequired):
		handlers.WriteError(w, http.StatusConflict, "account_link_required", "Use the account migration page once, then connect Keycloak from Settings")
		return
	case errors.Is(userErr, repository.ErrOIDCIdentityConflict):
		handlers.WriteError(w, http.StatusConflict, "identity_already_linked", "This social identity is linked to another account")
		return
	case errors.Is(userErr, repository.ErrOIDCLinkIntentInvalid):
		handlers.WriteError(w, http.StatusBadRequest, "link_intent_invalid", "The account-link request expired; start again from Settings")
		return
	case userErr != nil || user == nil:
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to start social session")
		return
	}
	a.issueSession(r.Context(), w, user)
}

func (a *AuthAPI) setOIDCLinkCookie(w http.ResponseWriter, value string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name: oidcLinkCookie, Value: value, Path: "/api/v1/auth/oidc/session",
		Expires: expiresAt, MaxAge: int(oidcLinkTTL.Seconds()), HttpOnly: true,
		Secure: a.cfg.Environment == "production", SameSite: http.SameSiteLaxMode,
	})
}

func (a *AuthAPI) clearOIDCLinkCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: oidcLinkCookie, Value: "", Path: "/api/v1/auth/oidc/session",
		MaxAge: -1, HttpOnly: true, Secure: a.cfg.Environment == "production", SameSite: http.SameSiteLaxMode,
	})
}

func oidcUsername(identity authsvc.ExternalIdentity) string {
	source := identity.PreferredUsername
	if source == "" {
		source = identity.Name
	}
	if source == "" {
		source, _, _ = strings.Cut(identity.Email, "@")
	}
	var out strings.Builder
	for _, char := range strings.ToLower(source) {
		valid := char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '-'
		if valid {
			out.WriteRune(char)
		} else if out.Len() > 0 && !strings.HasSuffix(out.String(), "-") {
			out.WriteByte('-')
		}
		if out.Len() >= 30 {
			break
		}
	}
	username := strings.Trim(out.String(), "-_")
	if len(username) < 3 {
		return "player"
	}
	return username
}
