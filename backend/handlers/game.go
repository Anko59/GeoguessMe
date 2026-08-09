package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	chatHub "geoguessme/internal/chat"
	"geoguessme/internal/config"
	chatrepo "geoguessme/internal/repository/chat"
	"geoguessme/internal/repository/groups"
	"geoguessme/internal/storage"
)

type GuessRequest struct {
	Lat  float64 `json:"lat"`
	Long float64 `json:"long"`
}

// DeletionEnqueuer is the durable media-deletion seam used by upload
// compensation: when a database failure leaves stored media orphaned, the
// caller enqueues a deletion job instead of dropping the object. It decouples
// the gameplay and chat handlers from the concrete repository package;
// *repository.Repository satisfies it.
type DeletionEnqueuer interface {
	EnqueueMediaDeletion(ctx context.Context, source string, keys []string) error
}

// GameAPI serves the gameplay slice from injected dependencies (PR 6): groups,
// challenges, guesses, media delivery, and leaderboard. It owns transport only:
// request parsing, authorization delegation through the canonical membership
// gate, service calls, and response writing. Persistence lives in
// internal/repository/groups; the object store, push notifier, realtime hub,
// and clock are injected. GameAPI replaced the package-level gameplay handlers
// and the RuntimeConfig, MediaStore, and Push globals they read.
type GameAPI struct {
	groups   *groups.Repository
	messages *chatrepo.Repository
	media    DeletionEnqueuer
	store    storage.ObjectStore
	cfg      *config.Config
	push     PushNotifier
	hub      *chatHub.Hub
	clock    func() time.Time
}

// NewGameAPI constructs the gameplay transport with its explicit dependencies.
func NewGameAPI(
	groupsRepo *groups.Repository,
	messages *chatrepo.Repository,
	media DeletionEnqueuer,
	store storage.ObjectStore,
	cfg *config.Config,
	push PushNotifier,
	hub *chatHub.Hub,
	clock func() time.Time,
) *GameAPI {
	return &GameAPI{groups: groupsRepo, messages: messages, media: media, store: store, cfg: cfg, push: push, hub: hub, clock: clock}
}

// requireMember is the canonical membership gate for the gameplay slice. Every
// group-scoped gameplay handler delegates membership decisions to it so no
// handler can implement a subtly different rule. It returns true when the user
// is a member; on failure it writes the error response and returns false.
//
// Membership-check failures map to 403 forbidden exactly as the pre-migration
// handlers did: auth.VerifyGroupMembership returned an error for both
// non-members and persistence failures, and every gameplay handler answered
// forbidden. The behavior is preserved and now centralized in one place.
func (a *GameAPI) requireMember(w http.ResponseWriter, r *http.Request, groupID, userID string) bool {
	if err := a.groups.RequireMember(r.Context(), groupID, userID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden", "You are not a member of this group")
		return false
	}
	return true
}

// SubmitChallengeGuess records a guess after the viewing window has closed and
// broadcasts the resolved challenge message over the realtime hub.
func (a *GameAPI) SubmitChallengeGuess(w http.ResponseWriter, r *http.Request) {
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
	now := a.clock()
	result, err := a.groups.SubmitGuess(r.Context(), photoID, GetUserIDFromContext(r), req.Lat, req.Long, now)
	if err != nil {
		switch {
		case errors.Is(err, groups.ErrForbidden):
			writeError(w, http.StatusForbidden, "forbidden", "You cannot guess this challenge")
		case errors.Is(err, groups.ErrOwnPhoto):
			writeError(w, http.StatusForbidden, "forbidden", "You cannot guess your own challenge")
		case errors.Is(err, groups.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "Challenge not found")
		case errors.Is(err, groups.ErrChallengeExpired):
			writeError(w, http.StatusGone, "challenge_expired", "This challenge has expired")
		case errors.Is(err, groups.ErrViewNotFinished):
			writeError(w, http.StatusConflict, "viewing_window_open", "Wait until the viewing window ends before guessing")
		case errors.Is(err, groups.ErrInvalidCoordinate):
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
	if !result.Existing && a.hub != nil {
		message, messageErr := a.messages.GetChallengeMessageForViewer(r.Context(), photoID, "")
		if messageErr != nil {
			slog.Error("failed to load challenge message after guess", "photo_id", photoID, "error", messageErr)
		} else if message != nil {
			message.ChallengeResolved = true
			a.hub.BroadcastUpdate(*message)
		}
	}
	writeJSON(w, status, map[string]any{"guess_id": result.Guess.ID, "photo_id": result.Guess.PhotoID, "score": result.Guess.Score, "distance": result.Guess.Distance, "created_at": result.Guess.CreatedAt, "duplicate": result.Existing, "server_time": now})
}

// GetChallengeResults returns the leaderboard of a challenge, hiding other
// players' exact points while a hidden-location challenge is still masked.
func (a *GameAPI) GetChallengeResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	photoID := r.PathValue("photoID")
	now := a.clock()
	viewerID := GetUserIDFromContext(r)
	photo, allowed, err := a.groups.CanViewResults(r.Context(), photoID, viewerID, now)
	if err != nil {
		if errors.Is(err, groups.ErrNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "Challenge not found")
		} else if errors.Is(err, groups.ErrForbidden) {
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
	guesses, err := a.groups.GuessesForPhoto(r.Context(), photoID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load results")
		return
	}
	// A poster who hid the location keeps the exact spot private from guessers
	// until the hide duration has passed; the owner always sees their own spot.
	// While hidden, other players' guessed points and distances are not sent at
	// all (score-only), and only the viewer's own guessed point is returned.
	hidden := photo.HideLocation && photo.UserID != viewerID && now.Before(photo.CreatedAt.Add(a.cfg.LocationHide))
	if guesses == nil {
		guesses = []groups.GuessWithUser{}
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
	response := map[string]any{"photo_id": photo.ID, "group_id": photo.GroupID, "guesses": responseGuesses, "media_available": photo.LifecycleStatus != "removed", "server_time": now}
	if hidden {
		response["location_hidden"] = true
		response["location_reveals_at"] = photo.CreatedAt.Add(a.cfg.LocationHide)
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
