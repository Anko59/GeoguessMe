package handlers

import (
	"errors"
	"log/slog"
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
	if !result.Existing && HubInstance != nil {
		message, messageErr := repository.GetChallengeMessageForViewer(r.Context(), photoID, "")
		if messageErr != nil {
			slog.Error("failed to load challenge message after guess", "photo_id", photoID, "error", messageErr)
		} else if message != nil {
			message.ChallengeResolved = true
			HubInstance.BroadcastUpdate(*message)
		}
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
	// A poster who hid the location keeps the exact spot private from guessers
	// until the hide duration has passed; the owner always sees their own spot.
	// While hidden, other players' guessed points and distances are not sent at
	// all (score-only), and only the viewer's own guessed point is returned.
	viewerID := GetUserIDFromContext(r)
	hidden := photo.HideLocation && photo.UserID != viewerID && time.Now().Before(photo.CreatedAt.Add(RuntimeConfig.LocationHide))
	if guesses == nil {
		guesses = []repository.GuessWithUser{}
	}
	responseGuesses := make([]resultsGuess, 0, len(guesses))
	for _, guess := range guesses {
		item := resultsGuess{
			ID:        guess.ID,
			PhotoID:   guess.PhotoID,
			UserID:    guess.UserID,
			GroupID:   guess.GroupID,
			Score:     guess.Score,
			CreatedAt: guess.CreatedAt,
			Username:  guess.Username,
			Avatar:    guess.Avatar,
		}
		if !hidden || guess.UserID == viewerID {
			item.Lat = &guess.Lat
			item.Long = &guess.Long
			item.Distance = &guess.Distance
		}
		responseGuesses = append(responseGuesses, item)
	}
	response := map[string]any{"photo_id": photo.ID, "group_id": photo.GroupID, "guesses": responseGuesses, "media_available": photo.LifecycleStatus != "removed", "server_time": time.Now()}
	if hidden {
		response["location_hidden"] = true
		response["location_reveals_at"] = photo.CreatedAt.Add(RuntimeConfig.LocationHide)
	} else {
		response["actual_lat"] = photo.Lat
		response["actual_long"] = photo.Long
	}
	if photo.LifecycleStatus != "removed" {
		response["media_url"] = mediaURL(photo, true)
		response["media_type"] = photo.MIMEType
	}
	writeJSON(w, http.StatusOK, response)
}

// resultsGuess is the results payload for a single guess. Lat, long, and
// distance are optional and omitted while a hidden-location challenge keeps
// other players' guessed points private.
type resultsGuess struct {
	ID        string    `json:"id"`
	PhotoID   string    `json:"photo_id"`
	UserID    string    `json:"user_id"`
	GroupID   string    `json:"group_id"`
	Lat       *float64  `json:"lat,omitempty"`
	Long      *float64  `json:"long,omitempty"`
	Score     int       `json:"score"`
	Distance  *float64  `json:"distance,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	Username  string    `json:"username"`
	Avatar    string    `json:"avatar"`
}
