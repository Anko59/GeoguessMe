package handlers

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"geoguessme/internal/media"
	"geoguessme/internal/repository"
	"geoguessme/internal/storage"
)

const customAvatarMarker = "custom"

func avatarStorageKey(userID string) string { return "avatars/user/" + userID }

// IsCustomAvatar reports whether the avatar string represents a user-uploaded
// photo rather than one of the built-in default avatars.
func IsCustomAvatar(avatar string) bool { return avatar == customAvatarMarker }

func UploadAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if MediaStore == nil || RuntimeConfig == nil {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable", "Avatar storage is unavailable")
		return
	}
	userID := GetUserIDFromContext(r)
	// Avatars are resized to a small thumbnail server-side, but the upload must
	// still accept a full-resolution phone photo. Reuse the shared runtime
	// upload limit (same as challenge uploads) instead of a separate cap that
	// rejected ordinary 3-8 MiB phone photos before normalization could shrink
	// them.
	maxBytes := RuntimeConfig.UploadMaxBytes
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024*1024)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload", "Upload is too large or malformed")
		return
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_photo", "A photo is required")
		return
	}
	defer file.Close()
	normalized, err := media.NormalizeAvatar(file, header.Size, maxBytes, RuntimeConfig.UploadMaxPixels)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_avatar", err.Error())
		return
	}
	key := avatarStorageKey(userID)
	current, err := repository.GetUserByID(r.Context(), userID)
	if err != nil || current == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load current avatar")
		return
	}
	var previous []byte
	if IsCustomAvatar(current.Avatar) {
		object, getErr := MediaStore.Get(r.Context(), key)
		if getErr == nil {
			previous, getErr = io.ReadAll(io.LimitReader(object, maxBytes+1))
			closeErr := object.Close()
			if getErr == nil {
				getErr = closeErr
			}
		}
		if getErr != nil && !errors.Is(getErr, storage.ErrObjectNotFound) {
			writeError(w, http.StatusBadGateway, "storage_error", "Unable to preserve current avatar")
			return
		}
	}
	if err := MediaStore.Put(r.Context(), key, bytes.NewReader(normalized.Data), int64(len(normalized.Data)), "image/jpeg"); err != nil {
		writeError(w, http.StatusBadGateway, "storage_error", "Unable to store avatar")
		return
	}
	if err := repository.SetUserAvatar(r.Context(), userID, customAvatarMarker); err != nil {
		var compensationErr error
		if previous != nil {
			compensationErr = MediaStore.Put(r.Context(), key, bytes.NewReader(previous), int64(len(previous)), "image/jpeg")
		} else {
			compensationErr = MediaStore.Delete(r.Context(), key)
		}
		if compensationErr != nil {
			slog.Error("failed to restore avatar after database error", "user_id", userID, "storage_key", key, "compensation_error", compensationErr)
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to save avatar")
		return
	}
	updated, err := repository.GetUserByID(r.Context(), userID)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to fetch user profile")
		return
	}
	writeJSON(w, http.StatusOK, userResponse(updated))
}

func ServeUserAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if MediaStore == nil {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable", "Avatar storage is unavailable")
		return
	}
	userID := r.PathValue("userID")
	if err := validateID(userID, "userID"); err != nil {
		writeError(w, http.StatusBadRequest, "missing_user_id", "User ID is required")
		return
	}
	user, err := repository.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if user.Avatar != customAvatarMarker {
		writeError(w, http.StatusNotFound, "not_found", "No custom avatar")
		return
	}
	object, err := MediaStore.Get(r.Context(), avatarStorageKey(userID))
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "No custom avatar")
			return
		}
		writeError(w, http.StatusBadGateway, "storage_error", "Unable to read avatar")
		return
	}
	defer object.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, object)
}
