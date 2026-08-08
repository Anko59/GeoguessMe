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
	chatHub "geoguessme/internal/chat"
	"geoguessme/internal/database"
	"geoguessme/internal/media"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	"geoguessme/internal/storage"
	"geoguessme/internal/validation"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// UploadPhoto is a handler factory over the challenge slice (PR 6 migrates it
// onto a GameAPI). The realtime hub is injected explicitly so the upload can
// broadcast the fresh challenge message to the group without a package global.
func UploadPhoto(hub *chatHub.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		groupIDs, err := challengeGroupIDs(r, userID)
		if err != nil {
			if errors.Is(err, errNotGroupMember) {
				writeError(w, http.StatusForbidden, "forbidden", "You are not a member of this group")
				return
			}
			writeError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
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
		hideLocation := strings.EqualFold(strings.TrimSpace(r.FormValue("hide_location")), "true")
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
		photos := make([]*models.Photo, 0, len(groupIDs))
		keys := make([]string, 0, len(groupIDs))
		// Each target group gets its own storage object and photo row so the
		// independent challenges share nothing (media deletion for one group can
		// never break another).
		for _, groupID := range groupIDs {
			key := "photos/" + uuid.NewString()
			if err := MediaStore.Put(r.Context(), key, bytes.NewReader(normalized.Data), int64(len(normalized.Data)), normalized.MIMEType); err != nil {
				compensateMediaDeletes(r, keys)
				writeError(w, http.StatusBadGateway, "storage_error", "Unable to store media")
				return
			}
			keys = append(keys, key)
			photos = append(photos, &models.Photo{ID: uuid.NewString(), UserID: userID, GroupID: groupID, StorageKey: key, MIMEType: normalized.MIMEType, ByteSize: int64(len(normalized.Data)), Lat: lat, Long: long, LifecycleStatus: "ready", HideLocation: hideLocation, CreatedAt: now, ExpiresAt: now.Add(RuntimeConfig.ChallengeTTL), RetentionAt: now.Add(RuntimeConfig.PhotoRetention)})
		}
		if err := repository.CreatePhotosContext(r.Context(), photos); err != nil {
			compensateMediaDeletes(r, keys)
			writeError(w, http.StatusInternalServerError, "internal_error", "Unable to create challenge")
			return
		}
		for _, photo := range photos {
			if hub != nil {
				photoIDCopy := photo.ID
				hub.Broadcast(models.Message{ID: uuid.NewString(), GroupID: photo.GroupID, UserID: userID, Kind: "challenge", PhotoID: &photoIDCopy, Content: "", CreatedAt: now})
			}
			if Push != nil {
				Push.NotifyNewChallenge(r.Context(), photo.GroupID, userID, photo.ID)
			}
		}
		first := photos[0]
		response := map[string]any{"id": first.ID, "group_id": first.GroupID, "expires_at": first.ExpiresAt, "created_at": now, "server_time": now}
		photoSummaries := make([]map[string]any, 0, len(photos))
		for _, photo := range photos {
			photoSummaries = append(photoSummaries, map[string]any{"id": photo.ID, "group_id": photo.GroupID})
		}
		response["photos"] = photoSummaries
		writeJSON(w, http.StatusCreated, response)
	}
}

// errNotGroupMember distinguishes a membership failure from an invalid id so
// the handler can answer 403 instead of 400.
var errNotGroupMember = errors.New("not a group member")

// challengeGroupIDs resolves the target groups for an upload: repeated
// group_ids form fields (comma-separated values accepted) with a fallback to
// the legacy single group_id field. The list is validated, deduplicated, and
// every group must be one the user belongs to.
func challengeGroupIDs(r *http.Request, userID string) ([]string, error) {
	var ids []string
	for _, value := range r.Form["group_ids"] {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				ids = append(ids, part)
			}
		}
	}
	if len(ids) == 0 {
		if single := strings.TrimSpace(r.FormValue("group_id")); single != "" {
			ids = append(ids, single)
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("group_id is required")
	}
	seen := make(map[string]bool, len(ids))
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := validateID(id, "group_id"); err != nil {
			return nil, err
		}
		if err := auth.VerifyGroupMembership(r.Context(), id, userID); err != nil {
			return nil, errNotGroupMember
		}
		unique = append(unique, id)
	}
	return unique, nil
}

// compensateMediaDeletes removes stored media objects after a failed upload so
// no orphaned bytes are left behind.
func compensateMediaDeletes(r *http.Request, keys []string) {
	for _, key := range keys {
		if err := MediaStore.Delete(r.Context(), key); err != nil {
			if enqueueErr := repository.EnqueueMediaDeletion(r.Context(), "upload-compensation", []string{key}); enqueueErr != nil {
				slog.Error("failed to persist upload compensation", "storage_key", key, "delete_error", err, "enqueue_error", enqueueErr)
			} else {
				slog.Warn("queued upload compensation after storage delete failure", "storage_key", key, "error", err)
			}
		}
	}
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
