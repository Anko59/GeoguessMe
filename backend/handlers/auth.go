package handlers

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"geoguessme/internal/auth"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	"geoguessme/internal/validation"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type SignupRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthUser struct {
	ID              string     `json:"id"`
	Username        string     `json:"username"`
	Email           string     `json:"email"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	Avatar          string     `json:"avatar"`
}

type AuthResponse struct {
	AccessToken string   `json:"access_token"`
	ExpiresIn   int64    `json:"expires_in"`
	User        AuthUser `json:"user"`
}

func userResponse(user *models.User) AuthUser {
	return AuthUser{ID: user.ID, Username: user.Username, Email: user.Email, EmailVerifiedAt: user.EmailVerifiedAt, Avatar: user.Avatar}
}

// writeSession issues an access token bound to the user's current auth version,
// records the provided refresh token in a cookie, and writes the auth response.
// The refresh session must already be persisted by the caller (signup/login
// create it directly; refresh rotation creates it inside its transaction).
func writeSession(w http.ResponseWriter, user *models.User, refreshToken string) {
	accessToken, err := auth.GenerateAccessToken(user.ID, user.AuthVersion)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to start session")
		return
	}
	setRefreshCookie(w, refreshToken)
	writeJSON(w, http.StatusOK, AuthResponse{AccessToken: accessToken, ExpiresIn: int64(RuntimeConfig.AccessTokenTTL.Seconds()), User: userResponse(user)})
}

func newRefreshMaterial(userID string) (raw, hash, id string, expiresAt time.Time, err error) {
	raw, err = auth.GenerateOpaqueToken(48)
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	hash = auth.HashToken(raw)
	id = uuid.NewString()
	expiresAt = time.Now().Add(RuntimeConfig.RefreshTokenTTL)
	_ = userID
	return
}

func issueSession(ctx context.Context, w http.ResponseWriter, user *models.User) {
	raw, hash, id, expiresAt, err := newRefreshMaterial(user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to start session")
		return
	}
	if err := repository.CreateRefreshSession(ctx, repository.RefreshSession{ID: id, UserID: user.ID, ExpiresAt: expiresAt}, hash); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to start session")
		return
	}
	writeSession(w, user, raw)
}

func Signup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req SignupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	if err := validation.ValidateUsername(req.Username); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_username", err.Error())
		return
	}
	if err := validation.ValidateEmail(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_email", err.Error())
		return
	}
	if err := validation.ValidatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_password", err.Error())
		return
	}
	if user, err := repository.GetUserByUsernameContext(r.Context(), req.Username); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to create account")
		return
	} else if user != nil {
		writeError(w, http.StatusConflict, "username_taken", "Username is already in use")
		return
	}
	if user, err := repository.GetUserByEmailContext(r.Context(), req.Email); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to create account")
		return
	} else if user != nil {
		writeError(w, http.StatusConflict, "email_taken", "Email is already in use")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), configuredCost())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to create account")
		return
	}
	now := time.Now()
	user := &models.User{ID: uuid.NewString(), Username: req.Username, Email: req.Email, Password: string(hash), Avatar: randomAvatar(), CreatedAt: now, UpdatedAt: now}
	if err := repository.CreateUserContext(r.Context(), user); err != nil {
		writeError(w, http.StatusConflict, "account_exists", "Unable to create account with those details")
		return
	}
	if err := issueVerificationToken(r, user); err != nil {
		// Account creation and gameplay do not depend on SMTP availability.
		slog.Warn("verification delivery failed", "error", err, "user_id", user.ID)
	}
	issueSession(r.Context(), w, user)
}

func Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req LoginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := repository.GetUserByUsernameContext(r.Context(), strings.TrimSpace(req.Username))
	if err != nil || user == nil || !auth.CheckPasswordHash(req.Password, user.Password) {
		writeError(w, http.StatusUnauthorized, "authentication_failed", "Authentication failed")
		return
	}
	issueSession(r.Context(), w, user)
}

func Refresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Refresh session is invalid")
		return
	}
	raw, hash, id, expiresAt, err := newRefreshMaterial("")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to refresh session")
		return
	}
	// Rotation retires the presented session and installs the replacement in a
	// single transaction; a nil user means the token was invalid or revoked.
	user, err := repository.RotateRefreshSession(r.Context(), auth.HashToken(cookie.Value), id, hash, expiresAt, time.Now())
	if err != nil || user == nil {
		clearRefreshCookie(w)
		writeError(w, http.StatusUnauthorized, "unauthorized", "Refresh session is invalid")
		return
	}
	writeSession(w, user, raw)
}

func Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if cookie, err := r.Cookie("refresh_token"); err == nil && cookie.Value != "" {
		hash := auth.HashToken(cookie.Value)
		if r.URL.Query().Get("all") == "1" {
			if userID, _ := repository.UserIDByRefreshHash(r.Context(), hash); userID != "" {
				_ = repository.RevokeAllRefreshSessions(r.Context(), userID)
				_ = repository.BumpAuthVersion(r.Context(), userID)
			}
		} else {
			_ = repository.RevokeRefreshSessionByHash(r.Context(), hash)
		}
	}
	clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

type TokenRequest struct {
	Token string `json:"token"`
}

func RequestVerification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	userID := GetUserIDFromContext(r)
	user, err := repository.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	if user.EmailVerifiedAt == nil {
		_ = issueVerificationToken(r, user)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "If the account can receive mail, a verification link has been sent"})
}

func VerifyEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req TokenRequest
	if !decodeJSON(w, r, &req) || req.Token == "" {
		writeError(w, http.StatusBadRequest, "invalid_token", "Verification token is required")
		return
	}
	if err := repository.VerifyEmailTransaction(r.Context(), auth.HashToken(req.Token)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_token", "Verification token is invalid or expired")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Email verified"})
}

func ForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if user, _ := repository.GetUserByEmailContext(r.Context(), req.Email); user != nil {
		_ = issueResetToken(r, user)
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "If the email is registered, a reset link has been sent"})
}

func ResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := validation.ValidatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_password", err.Error())
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), configuredCost())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to reset password")
		return
	}
	// Consume-token + password update + auth-version bump + session revocation
	// happen atomically; a token can only be used once even on partial failure.
	if err := repository.ResetPasswordTransaction(r.Context(), auth.HashToken(req.Token), string(hash)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_token", "Reset token is invalid or expired")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Password reset"})
}

func DeleteAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	userID := GetUserIDFromContext(r)
	user, err := repository.GetUserByID(r.Context(), userID)
	if err != nil || user == nil || subtle.ConstantTimeCompare([]byte{boolByte(auth.CheckPasswordHash(req.Password, user.Password))}, []byte{1}) != 1 {
		writeError(w, http.StatusUnauthorized, "authentication_failed", "Password confirmation failed")
		return
	}
	if _, err := repository.DeleteUserCascade(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to delete account")
		return
	}
	clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}

func issueVerificationToken(r *http.Request, user *models.User) error {
	token, err := auth.GenerateOpaqueToken(32)
	if err != nil {
		return err
	}
	ttl := 24 * time.Hour
	if RuntimeConfig != nil && RuntimeConfig.VerificationTTL > 0 {
		ttl = RuntimeConfig.VerificationTTL
	}
	if err := repository.InsertOneTimeToken(r.Context(), "email_verification_tokens", uuid.NewString(), user.ID, auth.HashToken(token), time.Now().Add(ttl)); err != nil {
		return err
	}
	return Mailer.Send(user.Email, "Verify your GeoGuessMe email", tokenURL("verify-email", token))
}

func issueResetToken(r *http.Request, user *models.User) error {
	token, err := auth.GenerateOpaqueToken(32)
	if err != nil {
		return err
	}
	ttl := time.Hour
	if RuntimeConfig != nil && RuntimeConfig.ResetTTL > 0 {
		ttl = RuntimeConfig.ResetTTL
	}
	if err := repository.InsertOneTimeToken(r.Context(), "password_reset_tokens", uuid.NewString(), user.ID, auth.HashToken(token), time.Now().Add(ttl)); err != nil {
		return err
	}
	return Mailer.Send(user.Email, "Reset your GeoGuessMe password", tokenURL("reset-password", token))
}

func tokenURL(path, token string) string {
	base := "http://localhost:5173"
	if RuntimeConfig != nil && RuntimeConfig.PublicURL != "" {
		base = RuntimeConfig.PublicURL
	}
	return fmt.Sprintf("%s/%s?token=%s", strings.TrimRight(base, "/"), path, token)
}

func setRefreshCookie(w http.ResponseWriter, value string) {
	secure := RuntimeConfig != nil && strings.EqualFold(RuntimeConfig.Environment, "production")
	maxAge := 30 * 24 * 60 * 60
	if RuntimeConfig != nil {
		maxAge = int(RuntimeConfig.RefreshTokenTTL.Seconds())
	}
	http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: value, Path: "/api/v1/auth", MaxAge: maxAge, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
}

func clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: "", Path: "/api/v1/auth", MaxAge: -1, HttpOnly: true, Secure: RuntimeConfig != nil && strings.EqualFold(RuntimeConfig.Environment, "production"), SameSite: http.SameSiteLaxMode})
}

var availableAvatars = []string{"avatar.png", "avatar2.png", "avatar3.png", "avatar4.png", "avatar5.png", "avatar6.png", "avatar7.png", "avatar8.png", "avatar9.png", "avatar10.png"}

func randomAvatar() string {
	index, err := rand.Int(rand.Reader, big.NewInt(int64(len(availableAvatars))))
	if err != nil {
		return availableAvatars[0]
	}
	return availableAvatars[index.Int64()]
}

func configuredCost() int {
	cost := bcrypt.DefaultCost
	if RuntimeConfig != nil && RuntimeConfig.PasswordHashCost >= bcrypt.MinCost && RuntimeConfig.PasswordHashCost <= bcrypt.MaxCost {
		cost = RuntimeConfig.PasswordHashCost
	}
	return cost
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 256*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return false
	}
	return true
}
