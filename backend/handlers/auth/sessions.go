package auth

import (
	"context"
	"net/http"
	"strings"
	"time"

	"geoguessme/handlers"
	authsvc "geoguessme/internal/auth"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"

	"github.com/google/uuid"
)

// writeSession issues an access token bound to the user's current auth
// version, records the provided refresh token in a cookie, and writes the auth
// response. The refresh session must already be persisted by the caller
// (signup/login create it directly; refresh rotation creates it inside its
// transaction).
func (a *AuthAPI) writeSession(w http.ResponseWriter, user *models.User, refreshToken string) {
	accessToken, err := a.svc.GenerateAccessToken(user.ID, user.AuthVersion)
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to start session")
		return
	}
	a.setRefreshCookie(w, refreshToken)
	handlers.WriteJSON(w, http.StatusOK, AuthResponse{AccessToken: accessToken, ExpiresIn: int64(a.cfg.AccessTokenTTL.Seconds()), User: userResponse(user)})
}

func (a *AuthAPI) newRefreshMaterial(userID string) (raw, hash, id string, expiresAt time.Time, err error) {
	raw, err = authsvc.GenerateOpaqueToken(48)
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	hash = authsvc.HashToken(raw)
	id = uuid.NewString()
	expiresAt = time.Now().Add(a.cfg.RefreshTokenTTL)
	_ = userID
	return
}

func (a *AuthAPI) issueSession(ctx context.Context, w http.ResponseWriter, user *models.User) {
	raw, hash, id, expiresAt, err := a.newRefreshMaterial(user.ID)
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to start session")
		return
	}
	if err := a.repos.CreateRefreshSession(ctx, repository.RefreshSession{ID: id, UserID: user.ID, ExpiresAt: expiresAt}, hash); err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to start session")
		return
	}
	a.writeSession(w, user, raw)
}

func (a *AuthAPI) setRefreshCookie(w http.ResponseWriter, value string) {
	secure := strings.EqualFold(a.cfg.Environment, "production")
	maxAge := int(a.cfg.RefreshTokenTTL.Seconds())
	http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: value, Path: "/api/v1/auth", MaxAge: maxAge, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
}

func (a *AuthAPI) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: "", Path: "/api/v1/auth", MaxAge: -1, HttpOnly: true, Secure: strings.EqualFold(a.cfg.Environment, "production"), SameSite: http.SameSiteLaxMode})
}
