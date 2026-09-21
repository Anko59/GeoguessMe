package feed

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
	"unicode/utf8"

	"geoguessme/handlers"
	"geoguessme/internal/config"
	"geoguessme/internal/media"
	"geoguessme/internal/models"
	feedrepo "geoguessme/internal/repository/feed"
	"geoguessme/internal/storage"
	"geoguessme/internal/validation"

	"github.com/google/uuid"
)

const maxDescriptionRunes = 280

func (a *API) Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	if a.store == nil {
		handlers.WriteError(w, 503, "storage_unavailable", "Photo storage is unavailable")
		return
	}
	maxBytes := a.cfg.UploadMaxBytes
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024*1024)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		handlers.WriteError(w, 400, "invalid_upload", "Upload is too large or malformed")
		return
	}
	defer func() {
		if err := r.MultipartForm.RemoveAll(); err != nil {
			slog.Error("remove public upload temporary files", "error", err)
		}
	}()
	caption := strings.TrimSpace(r.FormValue("caption"))
	if !utf8.ValidString(caption) || utf8.RuneCountInString(caption) > maxDescriptionRunes {
		handlers.WriteError(w, 400, "invalid_caption", "Descriptions can contain up to 280 characters")
		return
	}
	audience := r.FormValue("audience")
	if audience == "" {
		audience = "public"
	}
	if audience != "public" && audience != "friends" {
		handlers.WriteError(w, 400, "invalid_audience", "Audience must be public or friends")
		return
	}
	hideLocation := strings.EqualFold(strings.TrimSpace(r.FormValue("hide_location")), "true")
	groupIDs, err := selectedGroupIDs(r)
	if err != nil {
		handlers.WriteError(w, 400, "invalid_groups", err.Error())
		return
	}
	idempotencyKey := strings.TrimSpace(r.FormValue("idempotency_key"))
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	if _, err := uuid.Parse(idempotencyKey); err != nil {
		handlers.WriteError(w, 400, "invalid_idempotency_key", "A valid publication key is required")
		return
	}
	lat, latErr := strconv.ParseFloat(r.FormValue("lat"), 64)
	long, longErr := strconv.ParseFloat(r.FormValue("long"), 64)
	if latErr != nil || longErr != nil || validation.ValidateCoordinates(lat, long) != nil {
		handlers.WriteError(w, 400, "invalid_coordinates", "Choose the photo's location")
		return
	}
	userID := handlers.GetUserIDFromContext(r)
	file, header, err := r.FormFile("photo")
	if err != nil {
		handlers.WriteError(w, 400, "missing_photo", "Choose a photo to publish")
		return
	}
	defer file.Close()
	normalized, err := media.NormalizeUpload(file, header.Size, maxBytes, a.cfg.UploadMaxPixels)
	if err != nil {
		handlers.WriteError(w, 400, "invalid_media", "Choose a valid JPG, PNG, or WebP photo within the upload limit")
		return
	}
	preview, err := media.FeedPreview(normalized.Data)
	if err != nil {
		writeError(w, err)
		return
	}
	now := a.clock()
	p := feedrepo.NewChallenge{ID: idempotencyKey, UserID: userID, Caption: caption, Audience: audience, GroupIDs: groupIDs,
		StorageKey: "public-challenges/" + idempotencyKey, MIMEType: normalized.MIMEType, Preview: preview, Lat: lat, Long: long, CreatedAt: now}
	photos := makeGroupPhotos(idempotencyKey, userID, groupIDs, normalized.MIMEType, int64(len(normalized.Data)), lat, long, hideLocation, now, a.cfg)
	var reservation feedrepo.PublicationReservation
	if a.publisher != nil {
		reservation, err = a.publisher.ReserveFeedChallenge(r.Context(), p, photos)
		if err != nil {
			writeError(w, err)
			return
		}
		if reservation.Existing() {
			writeChallengeResponse(w, http.StatusOK, idempotencyKey, reservation.GroupIDs())
			return
		}
	}
	keys := make([]string, 0, len(photos)+1)
	keys = append(keys, p.StorageKey)
	for _, photo := range photos {
		keys = append(keys, photo.StorageKey)
	}
	storedKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if err := a.store.Put(r.Context(), key, bytes.NewReader(normalized.Data), int64(len(normalized.Data)), normalized.MIMEType); err != nil {
			if reservation != nil {
				if rollbackErr := reservation.Rollback(r.Context()); rollbackErr != nil {
					slog.Error("rollback public publication reservation", "error", rollbackErr)
				}
			}
			// A failed Put may have written an object before returning its
			// error, so include the current key, but never enqueue objects that
			// were not attempted yet.
			a.compensateKeys(r.Context(), append(storedKeys, key))
			handlers.WriteError(w, 502, "storage_error", "Unable to store photo")
			return
		}
		storedKeys = append(storedKeys, key)
	}
	if a.publisher != nil {
		err = reservation.Create(r.Context(), p, photos)
	} else {
		err = a.repo.Create(r.Context(), p)
	}
	if err != nil {
		if reservation != nil {
			if rollbackErr := reservation.Rollback(r.Context()); rollbackErr != nil {
				slog.Error("rollback public publication reservation", "error", rollbackErr)
			}
		}
		a.compensateKeys(r.Context(), keys)
		writeError(w, err)
		return
	}
	for _, photo := range photos {
		if a.hub != nil {
			photoID := photo.ID
			a.hub.Broadcast(models.Message{ID: uuid.NewString(), GroupID: photo.GroupID, UserID: userID, Kind: "challenge", PhotoID: &photoID, Content: "", CreatedAt: now})
		}
		if a.push != nil {
			a.push.NotifyNewChallenge(r.Context(), photo.GroupID, userID, photo.ID)
		}
	}
	writeChallengeResponse(w, http.StatusCreated, p.ID, groupIDs)
}

func makeGroupPhotos(challengeID, userID string, groupIDs []string, mimeType string, byteSize int64, lat, long float64, hideLocation bool, now time.Time, cfg *config.Config) []*models.Photo {
	photos := make([]*models.Photo, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		photoID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(challengeID+"\x00"+groupID)).String()
		photos = append(photos, &models.Photo{
			ID:              photoID,
			UserID:          userID,
			GroupID:         groupID,
			StorageKey:      "photos/" + photoID,
			MIMEType:        mimeType,
			ByteSize:        byteSize,
			Lat:             lat,
			Long:            long,
			LifecycleStatus: "ready",
			HideLocation:    hideLocation,
			CreatedAt:       now,
			ExpiresAt:       now.Add(cfg.ChallengeTTL),
			RetentionAt:     now.Add(cfg.PhotoRetention),
		})
	}
	return photos
}

func writeChallengeResponse(w http.ResponseWriter, status int, challengeID string, groupIDs []string) {
	photos := make([]map[string]string, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		photos = append(photos, map[string]string{
			"id":       uuid.NewSHA1(uuid.NameSpaceURL, []byte(challengeID+"\x00"+groupID)).String(),
			"group_id": groupID,
		})
	}
	handlers.WriteJSON(w, status, map[string]any{"id": challengeID, "photos": photos})
}

func selectedGroupIDs(r *http.Request) ([]string, error) {
	values := r.Form["group_id"]
	if len(values) > 20 {
		return nil, errors.New("A post can target at most 20 groups")
	}
	ids := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		id := strings.TrimSpace(value)
		if _, err := uuid.Parse(id); err != nil {
			return nil, errors.New("Each group ID must be a valid UUID")
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func (a *API) compensate(ctx context.Context, key string) {
	a.compensateKeys(ctx, []string{key})
}

func (a *API) compensateKeys(ctx context.Context, keys []string) {
	for _, key := range keys {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		err := a.store.Delete(cleanup, key)
		cancel()
		if err == nil {
			continue
		}
		slog.Error("public upload compensation failed", "storage_key", key, "error", err)
		// Storage may have exhausted its deadline. Give the durable fallback
		// its own budget so an object-store timeout cannot cancel the enqueue.
		queued, cancelQueue := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		if a.deletions == nil {
			cancelQueue()
			continue
		}
		if enqueueErr := a.deletions.EnqueueMediaDeletion(queued, "manual", []string{key}); enqueueErr != nil {
			slog.Error("enqueue public upload deletion failed", "storage_key", key, "error", enqueueErr)
		}
		cancelQueue()
	}
}

func (a *API) Media(w http.ResponseWriter, r *http.Request) { a.serveMedia(w, r, false) }

// Play explicitly opens the original photo for an untimed public attempt.
// Feed browsing always uses Media and stays blurred until a guess is recorded.
func (a *API) Play(w http.ResponseWriter, r *http.Request) { a.serveMedia(w, r, true) }

func (a *API) serveMedia(w http.ResponseWriter, r *http.Request, playing bool) {
	if r.Method != http.MethodGet {
		handlers.MethodNotAllowed(w)
		return
	}
	if !validID(w, r) {
		return
	}
	asset, err := a.repo.Media(r.Context(), r.PathValue("id"), handlers.GetUserIDFromContext(r))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if !playing && !asset.Revealed {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Length", strconv.Itoa(len(asset.Preview)))
		if _, err := w.Write(asset.Preview); err != nil {
			slog.Warn("write public preview", "error", err)
		}
		return
	}
	if a.store == nil {
		handlers.WriteError(w, 503, "storage_unavailable", "Photo storage is unavailable")
		return
	}
	body, err := a.store.Get(r.Context(), asset.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			handlers.WriteError(w, 410, "media_removed", "This photo is no longer available")
		} else {
			writeError(w, err)
		}
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", asset.MIMEType)
	if _, err := io.Copy(w, body); err != nil {
		slog.Warn("stream public photo", "error", err)
	}
}
