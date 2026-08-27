package auth

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"geoguessme/handlers"
	"geoguessme/internal/media"
	"geoguessme/internal/storage"
)

const customAvatarMarker = "custom"

func avatarStorageKey(userID string) string { return "avatars/user/" + userID }

// IsCustomAvatar reports whether the avatar string represents a user-uploaded
// photo rather than one of the built-in default avatars.
func IsCustomAvatar(avatar string) bool { return avatar == customAvatarMarker }

// UploadAvatar normalizes and stores a user-uploaded avatar photo, preserving
// the previous custom avatar on failure so no object is lost.
func (a *AuthAPI) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	if a.store == nil {
		handlers.WriteError(w, http.StatusServiceUnavailable, "storage_unavailable", "Avatar storage is unavailable")
		return
	}
	userID := handlers.GetUserIDFromContext(r)
	// Avatars are resized to a small thumbnail server-side, but the upload must
	// still accept a full-resolution phone photo. Avatar uploads have a larger
	// dedicated limit than challenge media because they are normalized to a
	// small thumbnail and do not consume the challenge-media budget.
	maxBytes := a.cfg.AvatarMaxBytes
	requestMaxBytes := maxBytes + 1024*1024
	if r.ContentLength > requestMaxBytes {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_upload", avatarTooLargeMessage(maxBytes))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, requestMaxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		if isRequestTooLarge(err) {
			handlers.WriteError(w, http.StatusBadRequest, "invalid_upload", avatarTooLargeMessage(maxBytes))
		} else {
			handlers.WriteError(w, http.StatusBadRequest, "invalid_upload", "We could not read that upload. Choose a JPG, PNG, or WebP image and try again.")
		}
		return
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "missing_photo", "A photo is required")
		return
	}
	defer file.Close()
	normalized, err := media.NormalizeAvatar(file, header.Size, maxBytes, a.cfg.UploadMaxPixels)
	if err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_avatar", err.Error())
		return
	}
	key := avatarStorageKey(userID)
	current, err := a.repos.GetUserByID(r.Context(), userID)
	if err != nil || current == nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load current avatar")
		return
	}
	var previous []byte
	if IsCustomAvatar(current.Avatar) {
		object, getErr := a.store.Get(r.Context(), key)
		if getErr == nil {
			previous, getErr = io.ReadAll(io.LimitReader(object, maxBytes+1))
			closeErr := object.Close()
			if getErr == nil {
				getErr = closeErr
			}
		}
		if getErr != nil && !errors.Is(getErr, storage.ErrObjectNotFound) {
			handlers.WriteError(w, http.StatusBadGateway, "storage_error", "Unable to preserve current avatar")
			return
		}
	}
	if err := a.store.Put(r.Context(), key, bytes.NewReader(normalized.Data), int64(len(normalized.Data)), "image/jpeg"); err != nil {
		handlers.WriteError(w, http.StatusBadGateway, "storage_error", "Unable to store avatar")
		return
	}
	if err := a.repos.SetUserAvatar(r.Context(), userID, customAvatarMarker); err != nil {
		var compensationErr error
		if previous != nil {
			compensationErr = a.store.Put(r.Context(), key, bytes.NewReader(previous), int64(len(previous)), "image/jpeg")
		} else {
			compensationErr = a.store.Delete(r.Context(), key)
		}
		if compensationErr != nil {
			slog.Error("failed to restore avatar after database error", "user_id", userID, "storage_key", key, "compensation_error", compensationErr)
		}
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to save avatar")
		return
	}
	updated, err := a.repos.GetUserByID(r.Context(), userID)
	if err != nil || updated == nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to fetch user profile")
		return
	}
	handlers.WriteJSON(w, http.StatusOK, a.userResponse(updated))
}

func isRequestTooLarge(err error) bool {
	var maxBytesError *http.MaxBytesError
	return errors.As(err, &maxBytesError) || strings.Contains(err.Error(), "message too large")
}

func avatarTooLargeMessage(maxBytes int64) string {
	return fmt.Sprintf("This photo is too large. Choose an image smaller than %d MiB.", maxBytes/(1024*1024))
}

// ServeUserAvatar streams a user's custom avatar, or 404 for default avatars.
func (a *AuthAPI) ServeUserAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		handlers.MethodNotAllowed(w)
		return
	}
	if a.store == nil {
		handlers.WriteError(w, http.StatusServiceUnavailable, "storage_unavailable", "Avatar storage is unavailable")
		return
	}
	userID := r.PathValue("userID")
	if err := handlers.ValidateID(userID, "userID"); err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "missing_user_id", "User ID is required")
		return
	}
	user, err := a.repos.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		handlers.WriteError(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if user.Avatar != customAvatarMarker {
		handlers.WriteError(w, http.StatusNotFound, "not_found", "No custom avatar")
		return
	}
	object, err := a.store.Get(r.Context(), avatarStorageKey(userID))
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			handlers.WriteError(w, http.StatusNotFound, "not_found", "No custom avatar")
			return
		}
		handlers.WriteError(w, http.StatusBadGateway, "storage_error", "Unable to read avatar")
		return
	}
	defer object.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, object)
}
