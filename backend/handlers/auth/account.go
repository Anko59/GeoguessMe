package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"geoguessme/handlers"
	authsvc "geoguessme/internal/auth"
	"geoguessme/internal/models"
	"geoguessme/internal/validation"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Signup creates an account, starts a session, and best-effort delivers a
// verification email. Account creation and gameplay do not depend on SMTP
// availability.
func (a *AuthAPI) Signup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	var req SignupRequest
	if !handlers.DecodeJSON(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	if err := validation.ValidateUsername(req.Username); err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_username", err.Error())
		return
	}
	if err := validation.ValidateEmail(req.Email); err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_email", err.Error())
		return
	}
	if err := validation.ValidatePassword(req.Password); err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_password", err.Error())
		return
	}
	if user, err := a.repos.GetUserByUsername(r.Context(), req.Username); err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create account")
		return
	} else if user != nil {
		handlers.WriteError(w, http.StatusConflict, "username_taken", "Username is already in use")
		return
	}
	if user, err := a.repos.GetUserByEmail(r.Context(), req.Email); err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create account")
		return
	} else if user != nil {
		handlers.WriteError(w, http.StatusConflict, "email_taken", "Email is already in use")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), a.configuredCost())
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create account")
		return
	}
	now := time.Now()
	user := &models.User{ID: uuid.NewString(), Username: req.Username, Email: req.Email, Password: string(hash), Avatar: a.randomAvatar(), CreatedAt: now, UpdatedAt: now}
	if err := a.repos.CreateUser(r.Context(), user); err != nil {
		handlers.WriteError(w, http.StatusConflict, "account_exists", "Unable to create account with those details")
		return
	}
	if err := a.issueVerificationToken(r, user); err != nil {
		// Account creation and gameplay do not depend on SMTP availability.
		slog.Warn("verification delivery failed", "error", err, "user_id", user.ID)
	}
	a.issueSession(r.Context(), w, user)
}

// Login verifies credentials and starts a session.
func (a *AuthAPI) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	var req LoginRequest
	if !handlers.DecodeJSON(w, r, &req) {
		return
	}
	user, err := a.repos.GetUserByUsername(r.Context(), strings.TrimSpace(req.Username))
	if err != nil || user == nil || !authsvc.CheckPasswordHash(req.Password, user.Password) {
		handlers.WriteError(w, http.StatusUnauthorized, "authentication_failed", "Authentication failed")
		return
	}
	a.issueSession(r.Context(), w, user)
}

// Refresh rotates the presented refresh session and installs a replacement.
func (a *AuthAPI) Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		handlers.WriteError(w, http.StatusUnauthorized, "unauthorized", "Refresh session is invalid")
		return
	}
	raw, hash, id, expiresAt, err := a.newRefreshMaterial("")
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to refresh session")
		return
	}
	// Rotation retires the presented session and installs the replacement in a
	// single transaction; a nil user means the token was invalid or revoked.
	user, err := a.repos.RotateRefreshSession(r.Context(), authsvc.HashToken(cookie.Value), id, hash, expiresAt, time.Now())
	if err != nil || user == nil {
		a.clearRefreshCookie(w)
		handlers.WriteError(w, http.StatusUnauthorized, "unauthorized", "Refresh session is invalid")
		return
	}
	a.writeSession(w, user, raw)
}

// Logout revokes the presented session (or every session when ?all=1) and
// clears the cookie.
func (a *AuthAPI) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	if cookie, err := r.Cookie("refresh_token"); err == nil && cookie.Value != "" {
		hash := authsvc.HashToken(cookie.Value)
		if r.URL.Query().Get("all") == "1" {
			if userID, _ := a.repos.UserIDByRefreshHash(r.Context(), hash); userID != "" {
				_ = a.repos.RevokeAllRefreshSessions(r.Context(), userID)
				_ = a.repos.BumpAuthVersion(r.Context(), userID)
			}
		} else {
			_ = a.repos.RevokeRefreshSessionByHash(r.Context(), hash)
		}
	}
	a.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// RequestVerification re-sends the email verification link for an unverified
// account.
func (a *AuthAPI) RequestVerification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	userID := handlers.GetUserIDFromContext(r)
	user, err := a.repos.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		handlers.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	if user.EmailVerifiedAt == nil {
		_ = a.issueVerificationToken(r, user)
	}
	handlers.WriteJSON(w, http.StatusAccepted, map[string]string{"message": "If the account can receive mail, a verification link has been sent"})
}

// VerifyEmail consumes a verification token and marks the account verified.
func (a *AuthAPI) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	var req TokenRequest
	if !handlers.DecodeJSON(w, r, &req) || req.Token == "" {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_token", "Verification token is required")
		return
	}
	if err := a.repos.VerifyEmailTransaction(r.Context(), authsvc.HashToken(req.Token)); err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_token", "Verification token is invalid or expired")
		return
	}
	handlers.WriteJSON(w, http.StatusOK, map[string]string{"message": "Email verified"})
}

// ForgotPassword sends a reset link when the email is registered. The response
// is identical whether or not the account exists.
func (a *AuthAPI) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if !handlers.DecodeJSON(w, r, &req) {
		return
	}
	if user, _ := a.repos.GetUserByEmail(r.Context(), req.Email); user != nil {
		_ = a.issueResetToken(r, user)
	}
	handlers.WriteJSON(w, http.StatusAccepted, map[string]string{"message": "If the email is registered, a reset link has been sent"})
}

// ResetPassword consumes a reset token and installs a new password.
func (a *AuthAPI) ResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !handlers.DecodeJSON(w, r, &req) {
		return
	}
	if err := validation.ValidatePassword(req.Password); err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_password", err.Error())
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), a.configuredCost())
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to reset password")
		return
	}
	// Consume-token + password update + auth-version bump + session revocation
	// happen atomically; a token can only be used once even on partial failure.
	if err := a.repos.ResetPasswordTransaction(r.Context(), authsvc.HashToken(req.Token), string(hash)); err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_token", "Reset token is invalid or expired")
		return
	}
	handlers.WriteJSON(w, http.StatusOK, map[string]string{"message": "Password reset"})
}

// DeleteAccount removes the account after password confirmation and clears the
// refresh cookie.
func (a *AuthAPI) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		handlers.MethodNotAllowed(w)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !handlers.DecodeJSON(w, r, &req) {
		return
	}
	userID := handlers.GetUserIDFromContext(r)
	user, err := a.repos.GetUserByID(r.Context(), userID)
	if err != nil || user == nil || subtle.ConstantTimeCompare([]byte{boolByte(authsvc.CheckPasswordHash(req.Password, user.Password))}, []byte{1}) != 1 {
		handlers.WriteError(w, http.StatusUnauthorized, "authentication_failed", "Password confirmation failed")
		return
	}
	if _, err := a.repos.DeleteUserCascade(r.Context(), userID); err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to delete account")
		return
	}
	a.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}

func (a *AuthAPI) issueVerificationToken(r *http.Request, user *models.User) error {
	token, err := authsvc.GenerateOpaqueToken(32)
	if err != nil {
		return err
	}
	ttl := 24 * time.Hour
	if a.cfg.VerificationTTL > 0 {
		ttl = a.cfg.VerificationTTL
	}
	if err := a.repos.InsertOneTimeToken(r.Context(), "email_verification_tokens", uuid.NewString(), user.ID, authsvc.HashToken(token), time.Now().Add(ttl)); err != nil {
		return err
	}
	return a.mailer.Send(user.Email, "Verify your GeoGuessMe email", a.tokenURL("verify-email", token))
}

func (a *AuthAPI) issueResetToken(r *http.Request, user *models.User) error {
	token, err := authsvc.GenerateOpaqueToken(32)
	if err != nil {
		return err
	}
	ttl := time.Hour
	if a.cfg.ResetTTL > 0 {
		ttl = a.cfg.ResetTTL
	}
	if err := a.repos.InsertOneTimeToken(r.Context(), "password_reset_tokens", uuid.NewString(), user.ID, authsvc.HashToken(token), time.Now().Add(ttl)); err != nil {
		return err
	}
	return a.mailer.Send(user.Email, "Reset your GeoGuessMe password", a.tokenURL("reset-password", token))
}

func (a *AuthAPI) tokenURL(path, token string) string {
	base := "http://localhost:5173"
	if a.cfg.PublicURL != "" {
		base = a.cfg.PublicURL
	}
	return fmt.Sprintf("%s/%s?token=%s", strings.TrimRight(base, "/"), path, token)
}

func (a *AuthAPI) configuredCost() int {
	cost := bcrypt.DefaultCost
	if a.cfg.PasswordHashCost >= bcrypt.MinCost && a.cfg.PasswordHashCost <= bcrypt.MaxCost {
		cost = a.cfg.PasswordHashCost
	}
	return cost
}

// defaultAvatarNames is the fixed set of built-in avatar names. It is a pure
// function returning a fresh slice so no package-level mutable state exists.
func defaultAvatarNames() []string {
	return []string{"avatar.png", "avatar2.png", "avatar3.png", "avatar4.png", "avatar5.png", "avatar6.png", "avatar7.png", "avatar8.png", "avatar9.png", "avatar10.png"}
}

func (a *AuthAPI) randomAvatar() string {
	names := defaultAvatarNames()
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(names))))
	if err != nil {
		return names[0]
	}
	return names[index.Int64()]
}

// isAvailableAvatar reports whether the requested avatar is one of the built-in
// defaults or a user-uploaded custom photo.
func isAvailableAvatar(avatar string) bool {
	if IsCustomAvatar(avatar) {
		return true
	}
	for _, candidate := range defaultAvatarNames() {
		if avatar == candidate {
			return true
		}
	}
	return false
}
