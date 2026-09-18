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
	"geoguessme/internal/media"
	"geoguessme/internal/repository/feed"
	"geoguessme/internal/storage"
	"geoguessme/internal/validation"

	"github.com/google/uuid"
)

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
	if !utf8.ValidString(caption) || utf8.RuneCountInString(caption) > 500 {
		handlers.WriteError(w, 400, "invalid_caption", "Captions can contain up to 500 characters")
		return
	}
	lat, latErr := strconv.ParseFloat(r.FormValue("lat"), 64)
	long, longErr := strconv.ParseFloat(r.FormValue("long"), 64)
	if latErr != nil || longErr != nil || validation.ValidateCoordinates(lat, long) != nil {
		handlers.WriteError(w, 400, "invalid_coordinates", "Choose the photo's location")
		return
	}
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
	p := feed.NewChallenge{ID: uuid.NewString(), UserID: handlers.GetUserIDFromContext(r), Caption: caption,
		StorageKey: "public-challenges/" + uuid.NewString(), MIMEType: normalized.MIMEType, Preview: preview, Lat: lat, Long: long, CreatedAt: a.clock()}
	if err := a.store.Put(r.Context(), p.StorageKey, bytes.NewReader(normalized.Data), int64(len(normalized.Data)), p.MIMEType); err != nil {
		a.compensate(r.Context(), p.StorageKey)
		handlers.WriteError(w, 502, "storage_error", "Unable to store photo")
		return
	}
	if err := a.repo.Create(r.Context(), p); err != nil {
		a.compensate(r.Context(), p.StorageKey)
		writeError(w, err)
		return
	}
	handlers.WriteJSON(w, 201, map[string]string{"id": p.ID})
}

func (a *API) compensate(ctx context.Context, key string) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	err := a.store.Delete(cleanup, key)
	cancel()
	if err != nil {
		slog.Error("public upload compensation failed", "storage_key", key, "error", err)
		// Storage may have exhausted its deadline. Give the durable fallback
		// its own budget so an object-store timeout cannot cancel the enqueue.
		queued, cancelQueue := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancelQueue()
		if err := a.deletions.EnqueueMediaDeletion(queued, "manual", []string{key}); err != nil {
			slog.Error("enqueue public upload deletion failed", "storage_key", key, "error", err)
		}
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
