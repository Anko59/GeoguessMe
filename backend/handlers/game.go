package handlers

import (
	"errors"
	"net/http"
	"time"

	"geoguessme/internal/repository"
)

type GuessRequest struct {
	Lat  float64 `json:"lat"`
	Long float64 `json:"long"`
}

func SubmitChallengeGuess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	photoID := r.PathValue("photoID")
	if err := validateID(photoID, "photo_id"); err != nil {
		writeError(w, http.StatusBadRequest, "missing_photo_id", "Photo ID is required")
		return
	}
	var req GuessRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	result, err := repository.SubmitGuess(r.Context(), photoID, GetUserIDFromContext(r), req.Lat, req.Long, time.Now())
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrForbidden):
			writeError(w, http.StatusForbidden, "forbidden", "You cannot guess this challenge")
		case errors.Is(err, repository.ErrOwnPhoto):
			writeError(w, http.StatusForbidden, "forbidden", "You cannot guess your own challenge")
		case errors.Is(err, repository.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "Challenge not found")
		case errors.Is(err, repository.ErrChallengeExpired):
			writeError(w, http.StatusGone, "challenge_expired", "This challenge has expired")
		case errors.Is(err, repository.ErrViewNotFinished):
			writeError(w, http.StatusConflict, "viewing_window_open", "Wait until the viewing window ends before guessing")
		case errors.Is(err, repository.ErrInvalidCoordinate):
			writeError(w, http.StatusBadRequest, "invalid_coordinates", "Coordinates are invalid")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "Unable to save guess")
		}
		return
	}
	status := http.StatusCreated
	if result.Existing {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"guess_id": result.Guess.ID, "photo_id": result.Guess.PhotoID, "score": result.Guess.Score, "distance": result.Guess.Distance, "created_at": result.Guess.CreatedAt, "duplicate": result.Existing, "server_time": time.Now()})
}

func GetChallengeResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	photoID := r.PathValue("photoID")
	photo, allowed, err := repository.CanViewResults(r.Context(), photoID, GetUserIDFromContext(r), time.Now())
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "Challenge not found")
		} else if errors.Is(err, repository.ErrForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", "Results are not available")
		} else {
			writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load results")
		}
		return
	}
	if !allowed {
		writeError(w, http.StatusForbidden, "results_not_available", "Results are not available yet")
		return
	}
	guesses, err := repository.GetGuessesForPhotoContext(r.Context(), photoID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load results")
		return
	}
	if guesses == nil {
		guesses = []repository.GuessWithUser{}
	}
	response := map[string]any{"photo_id": photo.ID, "group_id": photo.GroupID, "actual_lat": photo.Lat, "actual_long": photo.Long, "guesses": guesses, "media_available": photo.LifecycleStatus != "removed", "server_time": time.Now()}
	if photo.LifecycleStatus != "removed" {
		response["media_url"] = mediaURL(photo, true)
	}
	writeJSON(w, http.StatusOK, response)
}
