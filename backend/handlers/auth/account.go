package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"time"

	"geoguessme/handlers"
	authsvc "geoguessme/internal/auth"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
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
	if a.cfg.OIDCEnabled {
		handlers.WriteError(w, http.StatusGone, "legacy_signup_disabled", "Create accounts through Keycloak")
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
	if req.Email != "" {
		if err := validation.ValidateEmail(req.Email); err != nil {
			handlers.WriteError(w, http.StatusBadRequest, "invalid_email", err.Error())
			return
		}
	}
	if err := validation.ValidatePassword(req.Password); err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_password", err.Error())
		return
	}
	// Pay the configured password-hash cost before any registration-state
	// decision so username and verified-email collisions cannot be separated
	// from an available account by the missing bcrypt work.
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), a.configuredCost())
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create account")
		return
	}
	// Only the public username can collide at signup. An email is merely a
	// pending recovery claim—even when another account already verified that
	// address—so signup never reveals whether an address is registered. The
	// first account to verify owns it; later verification attempts fail with a
	// generic error.
	if other, err := a.repos.GetUserByUsername(r.Context(), req.Username); err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create account")
		return
	} else if other != nil {
		handlers.WriteError(w, http.StatusConflict, "signup_unavailable", "Unable to create an account with these details")
		return
	}
	now := time.Now()
	user := &models.User{ID: uuid.NewString(), Username: req.Username, PendingEmail: req.Email, Password: string(hash), Avatar: a.randomAvatar(), CreatedAt: now, UpdatedAt: now}
	if err := a.repos.CreateUser(r.Context(), user); err != nil {
		// A concurrent signup can still lose the username race.
		if errors.Is(err, repository.ErrUsernameConflict) {
			handlers.WriteError(w, http.StatusConflict, "signup_unavailable", "Unable to create an account with these details")
			return
		}
		slog.Error("signup account insert failed", "error", err)
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create account")
		return
	}
	if req.Email != "" {
		if err := a.issueVerificationToken(r, user, req.Email); err != nil {
			// Account creation and gameplay do not depend on SMTP availability.
			slog.Warn("verification delivery failed", "error", err, "user_id", user.ID)
		}
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
	if err != nil || user == nil || !a.legacyPasswordAvailable(user) || !authsvc.CheckPasswordHash(req.Password, user.Password) {
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
// clears the cookie. Fail-closed: a server-side revocation error surfaces as
// 500 with the cookie cleared, and a 204 is only ever returned when no
// server-side revocation error occurred.
func (a *AuthAPI) Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		// Nothing to revoke; clearing the cookie is a truthful no-op.
		a.clearRefreshCookie(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	hash := authsvc.HashToken(cookie.Value)
	if r.URL.Query().Get("all") == "1" {
		userID, err := a.repos.UserIDByRefreshHash(r.Context(), hash)
		if err != nil {
			slog.Error("logout-all user lookup failed", "error", err)
			a.clearRefreshCookie(w)
			handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to sign out")
			return
		}
		if userID != "" {
			if err := a.repos.RevokeAllCredentials(r.Context(), userID); err != nil {
				slog.Error("logout-all revocation failed", "error", err, "user_id", userID)
				a.clearRefreshCookie(w)
				handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to sign out")
				return
			}
			// Close every live socket: they were minted under an auth version
			// the revocation just invalidated.
			a.kickDisconnectUser(userID)
		}
		a.clearRefreshCookie(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err := a.repos.RevokeRefreshSessionByHash(r.Context(), hash); err != nil {
		slog.Error("logout revocation failed", "error", err)
		a.clearRefreshCookie(w)
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to sign out")
		return
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
	// Resend targets the current pending claim only. A verified account with
	// no pending claim has nothing left to verify and is a silent no-op; the
	// response stays uniform so the endpoint never reveals verification state.
	if user.EmailVerifiedAt == nil || user.PendingEmail != "" {
		if target := repository.ResendTargetEmail(user); target != nil {
			if err := a.issueVerificationToken(r, user, *target); err != nil {
				slog.Warn("verification resend failed", "error", err, "user_id", user.ID)
			}
		}
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
		// Claim conflicts (the address is already verified by another account)
		// surface as a generic verification error that reveals neither the
		// conflict nor the owning account. Nothing-to-promote is a successful
		// no-op handled by the repository (the token is consumed and committed).
		if errors.Is(err, repository.ErrClaimConflict) {
			handlers.WriteError(w, http.StatusBadRequest, "verification_failed", "Unable to verify the email address")
			return
		}
		if errors.Is(err, repository.ErrTokenInvalid) || errors.Is(err, repository.ErrUserNotFound) {
			handlers.WriteError(w, http.StatusBadRequest, "invalid_token", "Verification token is invalid or expired")
			return
		}
		slog.Error("email verification transaction failed", "error", err)
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to verify email")
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
	user, err := a.repos.GetUserByVerifiedEmail(r.Context(), req.Email)
	if err != nil {
		slog.Error("password recovery lookup failed", "error", err)
	} else if user != nil {
		if err := a.issueResetToken(r, user); err != nil {
			slog.Warn("password recovery delivery failed", "error", err, "user_id", user.ID)
		}
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
	userID, err := a.repos.ResetPasswordTransaction(r.Context(), authsvc.HashToken(req.Token), string(hash))
	if err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_token", "Reset token is invalid or expired")
		return
	}
	// Close every live socket: the reset bumped the auth version and revoked
	// every session, so sockets minted under the old version must close.
	a.kickDisconnectUser(userID)
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
		Password     string `json:"password"`
		Confirmation string `json:"confirmation"`
	}
	if !handlers.DecodeJSON(w, r, &req) {
		return
	}
	userID := handlers.GetUserIDFromContext(r)
	user, err := a.repos.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		handlers.WriteError(w, http.StatusUnauthorized, "authentication_failed", "Account confirmation failed")
		return
	}
	passwordConfirmation := a.legacyPasswordAvailable(user)
	confirmed := subtle.ConstantTimeCompare([]byte{boolByte(passwordConfirmation && authsvc.CheckPasswordHash(req.Password, user.Password))}, []byte{1}) == 1
	if !passwordConfirmation {
		confirmed = subtle.ConstantTimeCompare([]byte(strings.TrimSpace(req.Confirmation)), []byte(user.Username)) == 1
	}
	if !confirmed {
		handlers.WriteError(w, http.StatusUnauthorized, "authentication_failed", "Account confirmation failed")
		return
	}
	if user.OIDCLinked {
		if a.oidcAdmin == nil {
			handlers.WriteError(w, http.StatusServiceUnavailable, "identity_deletion_unavailable", "Unable to delete the Keycloak account right now")
			return
		}
		identities, identityErr := a.repos.OIDCIdentitiesByUserID(r.Context(), userID)
		if identityErr != nil || len(identities) == 0 {
			handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to delete account")
			return
		}
		for _, identity := range identities {
			if identityErr := a.oidcAdmin.DeleteIdentity(r.Context(), identity.Issuer, identity.Subject); identityErr != nil {
				handlers.WriteError(w, http.StatusBadGateway, "identity_deletion_failed", "Keycloak could not delete the identity; no GeoGuessMe data was removed")
				return
			}
		}
	}
	if _, err := a.repos.DeleteUserCascade(r.Context(), userID); err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to delete account")
		return
	}
	// The account no longer exists; close every socket it had open.
	a.kickDisconnectUser(userID)
	a.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func boolByte(value bool) byte {
	if value {
		return 1
	}
	return 0
}

func (a *AuthAPI) issueVerificationToken(r *http.Request, user *models.User, target string) error {
	token, err := authsvc.GenerateOpaqueToken(32)
	if err != nil {
		return err
	}
	ttl := 24 * time.Hour
	if a.cfg.VerificationTTL > 0 {
		ttl = a.cfg.VerificationTTL
	}
	if err := a.repos.InsertEmailVerificationToken(r.Context(), uuid.NewString(), user.ID, authsvc.HashToken(token), target, time.Now().Add(ttl)); err != nil {
		return err
	}
	return a.mailer.Send(target, "Verify your GeoGuessMe email", a.tokenURL("verify-email", token))
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
	if err := a.repos.InsertPasswordResetToken(r.Context(), uuid.NewString(), user.ID, authsvc.HashToken(token), time.Now().Add(ttl)); err != nil {
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
