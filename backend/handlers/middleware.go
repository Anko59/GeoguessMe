package handlers

import (
	"context"
	"net/http"
	"strings"

	"geoguessme/internal/auth"
	"geoguessme/internal/repository"
)

type contextKey string

const userIDKey contextKey = "userID"

// AuthMiddleware validates the access token and then confirms, against the
// database, that the account is still active and that the token's auth version
// matches the stored value. This is what makes password reset, account
// deletion, and explicit logout-all invalidate access immediately, even before
// the short-lived access JWT would have expired.
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value := strings.TrimSpace(r.Header.Get("Authorization"))
		parts := strings.SplitN(value, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		claims, err := auth.ValidateAccessToken(strings.TrimSpace(parts[1]))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		status, err := repository.GetUserAuthStatus(r.Context(), claims.UserID)
		if err != nil || !status.Active {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
			return
		}
		if claims.AuthVersion != status.AuthVersion {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Session revoked")
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func GetUserIDFromContext(r *http.Request) string {
	userID, _ := r.Context().Value(userIDKey).(string)
	return userID
}

// WithUserID returns a context carrying the authenticated user identifier. It is
// the inverse of GetUserIDFromContext and lets packages that own handlers wired
// behind AuthMiddleware (such as the push REST endpoints) populate a request
// context in tests without depending on the unexported context key.
func WithUserID(parent context.Context, userID string) context.Context {
	return context.WithValue(parent, userIDKey, userID)
}
