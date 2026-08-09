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

	"geoguessme/internal/media"
	"geoguessme/internal/models"
	"geoguessme/internal/repository/groups"
	"geoguessme/internal/storage"
	"geoguessme/internal/validation"

	"github.com/google/uuid"
)

// UploadPhoto accepts a validated image or browser-recorded video and creates
// one independent challenge per selected target group. Media is stored first
// and every failure path compensates the already-stored objects so no orphaned
// bytes are left behind.
func (a *GameAPI) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if a.store == nil || a.cfg == nil {
		WriteError(w, http.StatusServiceUnavailable, "storage_unavailable", "Photo storage is unavailable")
		return
	}
	userID := GetUserIDFromContext(r)
	maxBytes := a.cfg.UploadMaxBytes
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024*1024)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_upload", "Upload is too large or malformed")
		return
	}
	groupIDs, err := a.challengeGroupIDs(r, userID)
	if err != nil {
		if errors.Is(err, errNotGroupMember) {
			WriteError(w, http.StatusForbidden, "forbidden", "You are not a member of this group")
			return
		}
		WriteError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	lat, err := strconv.ParseFloat(r.FormValue("lat"), 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_latitude", "Latitude is invalid")
		return
	}
	long, err := strconv.ParseFloat(r.FormValue("long"), 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_longitude", "Longitude is invalid")
		return
	}
	if err := validation.ValidateCoordinates(lat, long); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_coordinates", err.Error())
		return
	}
	hideLocation := strings.EqualFold(strings.TrimSpace(r.FormValue("hide_location")), "true")
	file, header, err := r.FormFile("photo")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "missing_photo", "A photo or video is required")
		return
	}
	defer file.Close()
	normalized, err := media.NormalizeChallengeUpload(file, header.Size, maxBytes, a.cfg.UploadMaxPixels)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_media", err.Error())
		return
	}
	now := a.clock()
	photos := make([]*models.Photo, 0, len(groupIDs))
	keys := make([]string, 0, len(groupIDs))
	// Each target group gets its own storage object and photo row so the
	// independent challenges share nothing (media deletion for one group can
	// never break another).
	for _, groupID := range groupIDs {
		key := "photos/" + uuid.NewString()
		if err := a.store.Put(r.Context(), key, bytes.NewReader(normalized.Data), int64(len(normalized.Data)), normalized.MIMEType); err != nil {
			a.compensateMediaDeletes(r, keys)
			WriteError(w, http.StatusBadGateway, "storage_error", "Unable to store media")
			return
		}
		keys = append(keys, key)
		photos = append(photos, &models.Photo{ID: uuid.NewString(), UserID: userID, GroupID: groupID, StorageKey: key, MIMEType: normalized.MIMEType, ByteSize: int64(len(normalized.Data)), Lat: lat, Long: long, LifecycleStatus: "ready", HideLocation: hideLocation, CreatedAt: now, ExpiresAt: now.Add(a.cfg.ChallengeTTL), RetentionAt: now.Add(a.cfg.PhotoRetention)})
	}
	if err := a.groups.CreatePhotos(r.Context(), photos); err != nil {
		a.compensateMediaDeletes(r, keys)
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create challenge")
		return
	}
	for _, photo := range photos {
		if a.hub != nil {
			photoIDCopy := photo.ID
			a.hub.Broadcast(models.Message{ID: uuid.NewString(), GroupID: photo.GroupID, UserID: userID, Kind: "challenge", PhotoID: &photoIDCopy, Content: "", CreatedAt: now})
		}
		if a.push != nil {
			a.push.NotifyNewChallenge(r.Context(), photo.GroupID, userID, photo.ID)
		}
	}
	first := photos[0]
	response := map[string]any{"id": first.ID, "group_id": first.GroupID, "expires_at": first.ExpiresAt, "created_at": now, "server_time": now}
	photoSummaries := make([]map[string]any, 0, len(photos))
	for _, photo := range photos {
		photoSummaries = append(photoSummaries, map[string]any{"id": photo.ID, "group_id": photo.GroupID})
	}
	response["photos"] = photoSummaries
	WriteJSON(w, http.StatusCreated, response)
}

// errNotGroupMember distinguishes a membership failure from an invalid id so
// the handler can answer 403 instead of 400.
var errNotGroupMember = errors.New("not a group member")

// challengeGroupIDs resolves the target groups for an upload from the repeated
// group_ids form fields (comma-separated values accepted). The list is
// validated, deduplicated, and every group must be one the user belongs to
// (through the canonical membership gate). The legacy singular group_id input
// was removed by the compatibility-removal PR; callers must use group_ids.
func (a *GameAPI) challengeGroupIDs(r *http.Request, userID string) ([]string, error) {
	var ids []string
	for _, value := range r.Form["group_ids"] {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				ids = append(ids, part)
			}
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("group_ids is required")
	}
	seen := make(map[string]bool, len(ids))
	unique := make([]string, 0, len(ids))
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := ValidateID(id, "group_id"); err != nil {
			return nil, err
		}
		if err := a.groups.RequireMember(r.Context(), id, userID); err != nil {
			return nil, errNotGroupMember
		}
		unique = append(unique, id)
	}
	return unique, nil
}

// compensateMediaDeletes removes stored media objects after a failed upload so
// no orphaned bytes are left behind; a failed delete is enqueued as a durable
// deletion job instead of being dropped.
func (a *GameAPI) compensateMediaDeletes(r *http.Request, keys []string) {
	for _, key := range keys {
		if err := a.store.Delete(r.Context(), key); err != nil {
			if enqueueErr := a.media.EnqueueMediaDeletion(r.Context(), "upload-compensation", []string{key}); enqueueErr != nil {
				slog.Error("failed to persist upload compensation", "storage_key", key, "delete_error", err, "enqueue_error", enqueueErr)
			} else {
				slog.Warn("queued upload compensation after storage delete failure", "storage_key", key, "error", err)
			}
		}
	}
}

// AcceptChallenge opens the private viewing window for a member on a challenge
// they did not post. The membership, ownership, and expiry rules are enforced
// by the persistence layer inside the same transaction that records the view.
func (a *GameAPI) AcceptChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	photoID := r.PathValue("photoID")
	if err := ValidateID(photoID, "photo_id"); err != nil {
		WriteError(w, http.StatusBadRequest, "missing_photo_id", "Photo ID is required")
		return
	}
	photo, view, err := a.groups.AcceptChallenge(r.Context(), photoID, GetUserIDFromContext(r), a.cfg.ViewWindow, a.clock())
	if err != nil {
		challengeError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"photo_id":             photo.ID,
		"media_url":            mediaURL(photo, false),
		"media_type":           photo.MIMEType,
		"accepted_at":          view.AcceptedAt,
		"view_expires_at":      view.ViewExpiresAt,
		"guess_after":          view.ViewExpiresAt,
		"challenge_expires_at": photo.ExpiresAt,
		"server_time":          a.clock(),
	})
}

// ConfirmChallengeMediaDelivered acknowledges the full delivery of challenge
// media and starts the authoritative viewing window.
func (a *GameAPI) ConfirmChallengeMediaDelivered(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	photoID := r.PathValue("photoID")
	if err := ValidateID(photoID, "photo_id"); err != nil {
		WriteError(w, http.StatusBadRequest, "missing_photo_id", "Photo ID is required")
		return
	}
	expiresAt, err := a.groups.MarkMediaDelivered(r.Context(), photoID, GetUserIDFromContext(r), a.cfg.ViewWindow, a.clock())
	if err != nil {
		if errors.Is(err, groups.ErrForbidden) {
			WriteError(w, http.StatusForbidden, "forbidden", "Challenge media was not accepted")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to start the viewing window")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"view_expires_at": expiresAt, "guess_after": expiresAt, "server_time": a.clock()})
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

// ServeChallengeMedia streams challenge media once per accepted viewing
// window. The result variant requires results access; the normal variant
// enforces the never-received / window-still-open rule and starts the window
// at the first full delivery.
func (a *GameAPI) ServeChallengeMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	if a.store == nil {
		WriteError(w, http.StatusServiceUnavailable, "storage_unavailable", "Photo storage is unavailable")
		return
	}
	photoID := r.PathValue("photoID")
	if err := ValidateID(photoID, "photo_id"); err != nil {
		WriteError(w, http.StatusBadRequest, "missing_photo_id", "Photo ID is required")
		return
	}
	photo, err := a.groups.Photo(r.Context(), photoID)
	if err != nil || photo == nil {
		WriteError(w, http.StatusNotFound, "not_found", "Media not found")
		return
	}
	userID := GetUserIDFromContext(r)
	now := a.clock()
	if r.URL.Query().Get("result") == "1" {
		_, allowed, err := a.groups.CanViewResults(r.Context(), photoID, userID, now)
		if err != nil || !allowed {
			WriteError(w, http.StatusForbidden, "forbidden", "Media is not available")
			return
		}
	} else {
		if !a.requireMember(w, r, photo.GroupID, userID) {
			return
		}
		// A player may fetch the media while they have never received it in
		// full, or while their viewing window is still open. The window starts
		// at the first full delivery (see the extension below) so a slow
		// connection cannot consume it; a re-fetch after the window is always
		// denied, preserving the view-once guarantee.
		delivered, expiresAt, err := a.groups.ViewDeliveryStatus(r.Context(), photoID, userID)
		if err != nil {
			WriteError(w, http.StatusForbidden, "media_expired", "The viewing window has expired")
			return
		}
		if delivered {
			if !now.Before(expiresAt) {
				WriteError(w, http.StatusForbidden, "media_expired", "The viewing window has expired")
				return
			}
		} else if !now.Before(photo.ExpiresAt) {
			WriteError(w, http.StatusForbidden, "media_expired", "The viewing window has expired")
			return
		}
	}
	if photo.LifecycleStatus == "removed" {
		WriteError(w, http.StatusGone, "media_removed", "The original media is no longer available")
		return
	}
	// Get verifies an object before returning a reader. Avoid a separate Stat
	// round trip here: the S3 implementation already probes its lazy reader.
	object, err := a.store.Get(r.Context(), photo.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			WriteError(w, http.StatusGone, "media_removed", "The original media is no longer available")
			return
		}
		WriteError(w, http.StatusBadGateway, "storage_error", "Unable to read media")
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
		if _, err := a.groups.MarkMediaDelivered(ctx, photoID, userID, a.cfg.ViewWindow, now); err != nil {
			slog.Error("failed to start challenge view window after media delivery", "photo_id", photoID, "user_id", userID, "error", err)
		}
	}
}

func challengeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, groups.ErrForbidden), errors.Is(err, groups.ErrOwnPhoto):
		WriteError(w, http.StatusForbidden, "forbidden", "You cannot accept this challenge")
	case errors.Is(err, groups.ErrNotFound):
		WriteError(w, http.StatusNotFound, "not_found", "Challenge not found")
	case errors.Is(err, groups.ErrChallengeExpired):
		WriteError(w, http.StatusGone, "challenge_expired", "This challenge has expired")
	default:
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to process challenge")
	}
}

// ValidateID rejects empty or non-UUID path identifiers before repository calls.
func ValidateID(value, _ string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("id is required")
	}
	if _, err := uuid.Parse(value); err != nil {
		return errors.New("id must be a UUID")
	}
	return nil
}
