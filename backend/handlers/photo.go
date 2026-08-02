package handlers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"geoguessme/internal/auth"
	"geoguessme/internal/database"
	"geoguessme/internal/media"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	"geoguessme/internal/storage"
	"geoguessme/internal/validation"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func UploadPhoto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if MediaStore == nil || RuntimeConfig == nil {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable", "Photo storage is unavailable")
		return
	}
	userID := GetUserIDFromContext(r)
	maxBytes := RuntimeConfig.UploadMaxBytes
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024*1024)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload", "Upload is too large or malformed")
		return
	}
	groupID := strings.TrimSpace(r.FormValue("group_id"))
	if err := validateID(groupID, "group_id"); err != nil {
		writeError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	if err := auth.VerifyGroupMembership(r.Context(), groupID, userID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden", "You are not a member of this group")
		return
	}
	lat, err := strconv.ParseFloat(r.FormValue("lat"), 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_latitude", "Latitude is invalid")
		return
	}
	long, err := strconv.ParseFloat(r.FormValue("long"), 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_longitude", "Longitude is invalid")
		return
	}
	if err := validation.ValidateCoordinates(lat, long); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_coordinates", err.Error())
		return
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_photo", "A photo or video is required")
		return
	}
	defer file.Close()
	normalized, err := media.NormalizeChallengeUpload(file, header.Size, maxBytes, RuntimeConfig.UploadMaxPixels)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_media", err.Error())
		return
	}
	now := time.Now()
	photoID := uuid.NewString()
	key := "photos/" + uuid.NewString()
	if err := MediaStore.Put(r.Context(), key, bytes.NewReader(normalized.Data), int64(len(normalized.Data)), normalized.MIMEType); err != nil {
		writeError(w, http.StatusBadGateway, "storage_error", "Unable to store media")
		return
	}
	photo := &models.Photo{ID: photoID, UserID: userID, GroupID: groupID, StorageKey: key, MIMEType: normalized.MIMEType, ByteSize: int64(len(normalized.Data)), Lat: lat, Long: long, LifecycleStatus: "ready", CreatedAt: now, ExpiresAt: now.Add(RuntimeConfig.ChallengeTTL), RetentionAt: now.Add(RuntimeConfig.PhotoRetention)}
	if err := repository.CreatePhotoContext(r.Context(), photo); err != nil {
		if deleteErr := MediaStore.Delete(r.Context(), key); deleteErr != nil {
			if enqueueErr := repository.EnqueueMediaDeletion(r.Context(), "upload-compensation", []string{key}); enqueueErr != nil {
				slog.Error("failed to persist upload compensation", "storage_key", key, "delete_error", deleteErr, "enqueue_error", enqueueErr)
			} else {
				slog.Warn("queued upload compensation after storage delete failure", "storage_key", key, "error", deleteErr)
			}
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to create challenge")
		return
	}
	if HubInstance != nil {
		photoIDCopy := photo.ID
		HubInstance.Broadcast(models.Message{ID: uuid.NewString(), GroupID: groupID, UserID: userID, Kind: "challenge", PhotoID: &photoIDCopy, Content: "", CreatedAt: now})
	}
	if Push != nil {
		Push.NotifyNewChallenge(r.Context(), groupID, userID, photo.ID)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": photo.ID, "group_id": photo.GroupID, "expires_at": photo.ExpiresAt, "created_at": photo.CreatedAt, "server_time": now})
}

func AcceptChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	photoID := r.PathValue("photoID")
	if err := validateID(photoID, "photo_id"); err != nil {
		writeError(w, http.StatusBadRequest, "missing_photo_id", "Photo ID is required")
		return
	}
	photo, view, err := repository.AcceptChallenge(r.Context(), photoID, GetUserIDFromContext(r), RuntimeConfig.ViewWindow, time.Now())
	if err != nil {
		challengeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"photo_id":             photo.ID,
		"media_url":            mediaURL(photo, false),
		"media_type":           photo.MIMEType,
		"accepted_at":          view.AcceptedAt,
		"view_expires_at":      view.ViewExpiresAt,
		"guess_after":          view.ViewExpiresAt,
		"challenge_expires_at": photo.ExpiresAt,
		"server_time":          time.Now(),
	})
}

func ConfirmChallengeMediaDelivered(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	photoID := r.PathValue("photoID")
	if err := validateID(photoID, "photo_id"); err != nil {
		writeError(w, http.StatusBadRequest, "missing_photo_id", "Photo ID is required")
		return
	}
	expiresAt, err := repository.MarkChallengeMediaDelivered(r.Context(), photoID, GetUserIDFromContext(r), RuntimeConfig.ViewWindow, time.Now())
	if err != nil {
		if errors.Is(err, repository.ErrForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", "Challenge media was not accepted")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to start the viewing window")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"view_expires_at": expiresAt, "guess_after": expiresAt, "server_time": time.Now()})
}

// mediaURL always returns a same-origin, authenticated API path. Internal S3
// endpoints and object keys never reach browsers; the frontend fetches media as
// an authenticated blob and renders it through a short-lived object URL.
func mediaURL(photo *models.Photo, result bool) string {
	value := "/api/v1/challenges/" + photo.ID + "/media"
	if result {
		value += "?result=1"
	}
	return value
}

func ServeChallengeMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if MediaStore == nil {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable", "Photo storage is unavailable")
		return
	}
	photoID := r.PathValue("photoID")
	if err := validateID(photoID, "photo_id"); err != nil {
		writeError(w, http.StatusBadRequest, "missing_photo_id", "Photo ID is required")
		return
	}
	photo, err := repository.GetPhotoContext(r.Context(), photoID)
	if err != nil || photo == nil {
		writeError(w, http.StatusNotFound, "not_found", "Media not found")
		return
	}
	userID := GetUserIDFromContext(r)
	if r.URL.Query().Get("result") == "1" {
		_, allowed, err := repository.CanViewResults(r.Context(), photoID, userID, time.Now())
		if err != nil || !allowed {
			writeError(w, http.StatusForbidden, "forbidden", "Media is not available")
			return
		}
	} else {
		if err := auth.VerifyGroupMembership(r.Context(), photo.GroupID, userID); err != nil {
			writeError(w, http.StatusForbidden, "forbidden", "Media is not available")
			return
		}
		// A player may fetch the media while they have never received it in
		// full, or while their viewing window is still open. The window starts
		// at the first full delivery (see the extension below) so a slow
		// connection cannot consume it; a re-fetch after the window is always
		// denied, preserving the view-once guarantee.
		var delivered pgtype.Timestamptz
		var expiresAt time.Time
		err := database.DB.QueryRow(r.Context(), `SELECT media_delivered_at, view_expires_at FROM challenge_views WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&delivered, &expiresAt)
		if err != nil {
			writeError(w, http.StatusForbidden, "media_expired", "The viewing window has expired")
			return
		}
		if delivered.Valid {
			if !time.Now().Before(expiresAt) {
				writeError(w, http.StatusForbidden, "media_expired", "The viewing window has expired")
				return
			}
		} else if !time.Now().Before(photo.ExpiresAt) {
			writeError(w, http.StatusForbidden, "media_expired", "The viewing window has expired")
			return
		}
	}
	if photo.LifecycleStatus == "removed" {
		writeError(w, http.StatusGone, "media_removed", "The original media is no longer available")
		return
	}
	// Get verifies an object before returning a reader. Avoid a separate Stat
	// round trip here: the S3 implementation already probes its lazy reader.
	object, err := MediaStore.Get(r.Context(), photo.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			writeError(w, http.StatusGone, "media_removed", "The original media is no longer available")
			return
		}
		writeError(w, http.StatusBadGateway, "storage_error", "Unable to read media")
		return
	}
	defer object.Close()
	w.Header().Set("Content-Type", photo.MIMEType)
	w.Header().Set("Cache-Control", "private, no-store")
	// The viewing window starts at the first full delivery. Use a short-lived
	// background context because the request context may be canceled as soon as
	// the final response byte reaches the client. The client also acknowledges
	// delivery explicitly, making the transition recoverable and authoritative.
	n, copyErr := io.Copy(w, object)
	if copyErr == nil && n == photo.ByteSize {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := repository.MarkChallengeMediaDelivered(ctx, photoID, userID, RuntimeConfig.ViewWindow, time.Now()); err != nil {
			slog.Error("failed to start challenge view window after media delivery", "photo_id", photoID, "user_id", userID, "error", err)
		}
	}
}

func challengeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, repository.ErrForbidden), errors.Is(err, repository.ErrOwnPhoto):
		writeError(w, http.StatusForbidden, "forbidden", "You cannot accept this challenge")
	case errors.Is(err, repository.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Challenge not found")
	case errors.Is(err, repository.ErrChallengeExpired):
		writeError(w, http.StatusGone, "challenge_expired", "This challenge has expired")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to process challenge")
	}
}

// validateID rejects empty or non-UUID path identifiers before repository calls.
func validateID(value, _ string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("id is required")
	}
	if _, err := uuid.Parse(value); err != nil {
		return errors.New("id must be a UUID")
	}
	return nil
}
