package handlers

import (
	"net/http"
	"strings"

	"geoguessme/internal/auth"
	"geoguessme/internal/repository"
	"geoguessme/internal/validation"

	"golang.org/x/crypto/bcrypt"
)

type profileUpdateRequest struct {
	Username        string `json:"username"`
	Email           string `json:"email"`
	Avatar          string `json:"avatar"`
	CurrentPassword string `json:"current_password"`
}

type passwordChangeRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func UpdateProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		methodNotAllowed(w)
		return
	}
	var req profileUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	userID := GetUserIDFromContext(r)
	user, err := repository.GetUserByID(r.Context(), userID)
	if err != nil || user == nil || !auth.CheckPasswordHash(req.CurrentPassword, user.Password) {
		writeError(w, http.StatusUnauthorized, "authentication_failed", "Current password is incorrect")
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
	if !isAvailableAvatar(req.Avatar) {
		writeError(w, http.StatusBadRequest, "invalid_avatar", "Choose one of the available avatars")
		return
	}
	if other, lookupErr := repository.GetUserByUsernameContext(r.Context(), req.Username); lookupErr != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to update profile")
		return
	} else if other != nil && other.ID != userID {
		writeError(w, http.StatusConflict, "username_taken", "Username is already in use")
		return
	}
	if other, lookupErr := repository.GetUserByEmailContext(r.Context(), req.Email); lookupErr != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to update profile")
		return
	} else if other != nil && other.ID != userID {
		writeError(w, http.StatusConflict, "email_taken", "Email is already in use")
		return
	}
	updated, err := repository.UpdateProfile(r.Context(), userID, req.Username, req.Email, req.Avatar)
	if err != nil || updated == nil {
		writeError(w, http.StatusConflict, "profile_update_failed", "Unable to update profile")
		return
	}
	writeJSON(w, http.StatusOK, userResponse(updated))
}

func ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req passwordChangeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	userID := GetUserIDFromContext(r)
	user, err := repository.GetUserByID(r.Context(), userID)
	if err != nil || user == nil || !auth.CheckPasswordHash(req.CurrentPassword, user.Password) {
		writeError(w, http.StatusUnauthorized, "authentication_failed", "Current password is incorrect")
		return
	}
	if err := validation.ValidatePassword(req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_password", err.Error())
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), configuredCost())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to change password")
		return
	}
	if err := repository.ChangePassword(r.Context(), userID, string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to change password")
		return
	}
	clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func isAvailableAvatar(avatar string) bool {
	for _, candidate := range availableAvatars {
		if avatar == candidate {
			return true
		}
	}
	return false
}
