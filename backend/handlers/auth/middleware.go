package auth

import (
	"net/http"
	"strings"

	"geoguessme/handlers"
)

// AuthMiddleware validates the access token and then confirms, against the
// database, that the account is still active and that the token's auth version
// matches the stored value. This is what makes password reset, account
// deletion, and explicit logout-all invalidate access immediately, even before
// the short-lived access JWT would have expired.
func (a *AuthAPI) AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value := strings.TrimSpace(r.Header.Get("Authorization"))
		parts := strings.SplitN(value, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			handlers.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		claims, err := a.svc.ValidateAccessToken(strings.TrimSpace(parts[1]))
		if err != nil {
			handlers.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		status, err := a.repos.GetUserAuthStatus(r.Context(), claims.UserID)
		if err != nil || !status.Active {
			handlers.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		if claims.AuthVersion != status.AuthVersion {
			handlers.WriteError(w, http.StatusUnauthorized, "unauthorized", "Session revoked")
			return
		}
		ctx := handlers.WithUserID(r.Context(), claims.UserID)
		ctx = handlers.WithMigrationRequired(ctx, a.cfg.OIDCEnabled && !status.OIDCLinked)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// LegacyReadOnlyMiddleware prevents an unmigrated password account from
// changing application state. Reads, the Keycloak-link action, recovery-email
// delivery, and account deletion remain available while the player migrates.
func (a *AuthAPI) LegacyReadOnlyMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !handlers.MigrationRequired(r) || r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		allowed := r.Method == http.MethodPost && (r.URL.Path == "/api/v1/auth/oidc/link" || r.URL.Path == "/api/v1/auth/verify/request") ||
			r.Method == http.MethodDelete && r.URL.Path == "/api/v1/auth/account"
		if allowed {
			next.ServeHTTP(w, r)
			return
		}
		handlers.WriteError(w, http.StatusForbidden, "migration_required", "Connect this account to Keycloak before making changes")
	}
}
