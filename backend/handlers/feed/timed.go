package feed

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"geoguessme/handlers"
	"geoguessme/internal/game"
	feedrepo "geoguessme/internal/repository/feed"
	"geoguessme/internal/storage"
)

func (a *API) AcceptTimed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !validID(w, r) {
		if r.Method != http.MethodPost {
			handlers.MethodNotAllowed(w)
		}
		return
	}
	now := a.clock()
	view, err := a.repo.AcceptTimedChallenge(r.Context(), r.PathValue("id"), handlers.GetUserIDFromContext(r), a.cfg.ViewWindow, a.cfg.GuessWindow, now)
	if err != nil {
		writeError(w, err)
		return
	}
	handlers.WriteJSON(w, http.StatusOK, map[string]any{
		"challenge_id":        r.PathValue("id"),
		"media_url":           "/api/v1/feed/challenges/" + r.PathValue("id") + "/timed-media",
		"media_type":          view.MediaType,
		"accepted_at":         view.AcceptedAt,
		"view_expires_at":     view.ViewExpiresAt,
		"guess_after":         view.ViewExpiresAt,
		"guess_expires_at":    view.GuessExpiresAt,
		"score_grace_seconds": game.ScoreGraceSeconds(),
		"server_time":         now,
	})
}

// TimedMedia streams the original image for an accepted timed session. The
// client must acknowledge complete delivery explicitly; serving bytes does not
// move the server-owned deadlines.
func (a *API) TimedMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		handlers.MethodNotAllowed(w)
		return
	}
	if !validID(w, r) {
		return
	}
	asset, err := a.repo.TimedMedia(r.Context(), r.PathValue("id"), handlers.GetUserIDFromContext(r), a.clock())
	if err != nil {
		writeError(w, err)
		return
	}
	if a.store == nil {
		handlers.WriteError(w, http.StatusServiceUnavailable, "storage_unavailable", "Photo storage is unavailable")
		return
	}
	if !storage.IsCanonicalKey(asset.StorageKey) {
		handlers.WriteError(w, http.StatusGone, "media_removed", "The original media is no longer available")
		return
	}
	body, err := a.store.Get(r.Context(), asset.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			handlers.WriteError(w, http.StatusGone, "media_removed", "The original media is no longer available")
			return
		}
		writeError(w, err)
		return
	}
	defer body.Close()
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", asset.MIMEType)
	if _, err := io.Copy(w, body); err != nil {
		slog.Warn("stream timed public photo", "error", err)
	}
}

func (a *API) MediaDelivered(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	if !validID(w, r) {
		return
	}
	now := a.clock()
	view, err := a.repo.MarkTimedMediaDelivered(r.Context(), r.PathValue("id"), handlers.GetUserIDFromContext(r), a.cfg.ViewWindow, a.cfg.GuessWindow, now)
	if err != nil {
		writeError(w, err)
		return
	}
	handlers.WriteJSON(w, http.StatusOK, map[string]any{
		"view_expires_at":     view.ViewExpiresAt,
		"guess_after":         view.ViewExpiresAt,
		"guess_expires_at":    view.GuessExpiresAt,
		"score_grace_seconds": game.ScoreGraceSeconds(),
		"server_time":         now,
	})
}

func decodeTimedPoint(w http.ResponseWriter, r *http.Request) (float64, float64, bool) {
	var req struct {
		Lat  *float64 `json:"lat"`
		Long *float64 `json:"long"`
	}
	if !handlers.DecodeJSON(w, r, &req) {
		return 0, 0, false
	}
	if req.Lat == nil || req.Long == nil {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_coordinates", "Choose a valid location")
		return 0, 0, false
	}
	return *req.Lat, *req.Long, true
}

func (a *API) TimedGuess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	if !validID(w, r) {
		return
	}
	lat, long, ok := decodeTimedPoint(w, r)
	if !ok {
		return
	}
	now := a.clock()
	result, duplicate, err := a.repo.TimedGuess(r.Context(), r.PathValue("id"), handlers.GetUserIDFromContext(r), lat, long, now)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	if duplicate {
		status = http.StatusOK
	}
	writeTimedGuessStatus(w, status, result, duplicate, now)
}

func writeTimedGuessStatus(w http.ResponseWriter, status int, result feedrepo.TimedGuessResult, duplicate bool, now time.Time) {
	response := map[string]any{
		"guess_id":     result.ID,
		"challenge_id": result.ChallengeID,
		"score":        result.Score,
		"timed_out":    result.TimedOut,
		"created_at":   result.CreatedAt,
		"duplicate":    duplicate,
		"server_time":  now,
	}
	if !result.TimedOut {
		response["distance"] = result.Distance
	}
	handlers.WriteJSON(w, status, response)
}

func (a *API) TimedTimeout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	if !validID(w, r) {
		return
	}
	now := a.clock()
	result, duplicate, err := a.repo.TimedTimeout(r.Context(), r.PathValue("id"), handlers.GetUserIDFromContext(r), now)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusCreated
	if duplicate {
		status = http.StatusOK
	}
	writeTimedGuessStatus(w, status, result, duplicate, now)
}

func (a *API) TimedResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		handlers.MethodNotAllowed(w)
		return
	}
	if !validID(w, r) {
		return
	}
	result, err := a.repo.TimedResults(r.Context(), r.PathValue("id"), handlers.GetUserIDFromContext(r), a.clock())
	if err != nil {
		writeError(w, err)
		return
	}
	result.ServerTime = a.clock()
	w.Header().Set("Cache-Control", "private, no-store")
	handlers.WriteJSON(w, http.StatusOK, result)
}
