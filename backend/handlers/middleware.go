package handlers

import (
	"context"
	"geoguessme/internal/auth"
	"net/http"
)

// AuthMiddleware validates the JWT token and adds the user ID to the request context
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenString := r.Header.Get("Authorization")
		if tokenString == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		claims, err := auth.ValidateToken(tokenString)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Add user ID to context for downstream handlers
		ctx := context.WithValue(r.Context(), "userID", claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// GetUserIDFromContext extracts the user ID from the request context
func GetUserIDFromContext(r *http.Request) string {
	userID, ok := r.Context().Value("userID").(string)
	if !ok {
		return ""
	}
	return userID
}
