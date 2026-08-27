package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	chatHub "geoguessme/internal/chat"
	"geoguessme/internal/config"
	"geoguessme/internal/models"
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

// MediaProcessingStore is the persistence seam for asynchronous media
// processing jobs: creating a job when a video upload is quarantined and
// reading a job's status for its owner. *repository.Repository satisfies it.
type MediaProcessingStore interface {
	CreateProcessingJob(ctx context.Context, job *models.MediaProcessingJob) error
	GetProcessingJob(ctx context.Context, jobID, userID string) (*models.MediaProcessingJob, error)
}

// UploadMediaStore combines the durable media-deletion seam and the
// processing-job persistence seam so upload handlers keep a single injected
// dependency for every media lifecycle operation.
type UploadMediaStore interface {
	DeletionEnqueuer
	MediaProcessingStore
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
	media    UploadMediaStore
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
	media UploadMediaStore,
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
		WriteError(w, http.StatusForbidden, "forbidden", "You are not a member of this group")
		return false
	}
	return true
}

// SubmitChallengeGuess records a guess after the viewing window has closed and
// broadcasts the resolved challenge message over the realtime hub.
func (a *GameAPI) SubmitChallengeGuess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	photoID := r.PathValue("photoID")
	if err := ValidateID(photoID, "photo_id"); err != nil {
		WriteError(w, http.StatusBadRequest, "missing_photo_id", "Photo ID is required")
		return
	}
	var req GuessRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	now := a.clock()
	result, err := a.groups.SubmitGuess(r.Context(), photoID, GetUserIDFromContext(r), req.Lat, req.Long, now)
	if err != nil {
		switch {
		case errors.Is(err, groups.ErrForbidden):
			WriteError(w, http.StatusForbidden, "forbidden", "You cannot guess this challenge")
		case errors.Is(err, groups.ErrOwnPhoto):
			WriteError(w, http.StatusForbidden, "forbidden", "You cannot guess your own challenge")
		case errors.Is(err, groups.ErrNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "Challenge not found")
		case errors.Is(err, groups.ErrChallengeExpired):
			WriteError(w, http.StatusGone, "challenge_expired", "This challenge has expired")
		case errors.Is(err, groups.ErrViewNotFinished):
			WriteError(w, http.StatusConflict, "viewing_window_open", "Wait until the viewing window ends before guessing")
		case errors.Is(err, groups.ErrGuessTimeExpired):
			WriteError(w, http.StatusGone, "guess_time_expired", "You did not guess in time")
		case errors.Is(err, groups.ErrInvalidCoordinate):
			WriteError(w, http.StatusBadRequest, "invalid_coordinates", "Coordinates are invalid")
		default:
			WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to save guess")
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
	response := map[string]any{"guess_id": result.Guess.ID, "photo_id": result.Guess.PhotoID, "score": result.Guess.Score, "distance": result.Guess.Distance, "timed_out": result.Guess.TimedOut, "created_at": result.Guess.CreatedAt, "duplicate": result.Existing, "server_time": now}
	if result.PartyDoubled {
		// Surfaced only when true so the optional contract field keeps its
		// "party bonus applied" meaning for clients.
		response["party_doubled"] = true
	}
	WriteJSON(w, status, response)
}

// TimeoutChallengeGuess records a timed-out guess (score 0) when the
// server-authoritative guess window expired.
func (a *GameAPI) TimeoutChallengeGuess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	photoID := r.PathValue("photoID")
	if err := ValidateID(photoID, "photo_id"); err != nil {
		WriteError(w, http.StatusBadRequest, "missing_photo_id", "Photo ID is required")
		return
	}
	now := a.clock()
	result, err := a.groups.TimeoutGuess(r.Context(), photoID, GetUserIDFromContext(r), now)
	if err != nil {
		switch {
		case errors.Is(err, groups.ErrForbidden):
			WriteError(w, http.StatusForbidden, "forbidden", "You cannot guess this challenge")
		case errors.Is(err, groups.ErrOwnPhoto):
			WriteError(w, http.StatusForbidden, "forbidden", "You cannot guess your own challenge")
		case errors.Is(err, groups.ErrNotFound):
			WriteError(w, http.StatusNotFound, "not_found", "Challenge not found")
		case errors.Is(err, groups.ErrChallengeExpired):
			WriteError(w, http.StatusGone, "challenge_expired", "This challenge has expired")
		case errors.Is(err, groups.ErrViewNotFinished):
			WriteError(w, http.StatusConflict, "viewing_window_open", "Wait until the viewing window ends before guessing")
		default:
			WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to save guess")
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
			slog.Error("failed to load challenge message after timeout", "photo_id", photoID, "error", messageErr)
		} else if message != nil {
			message.ChallengeResolved = true
			a.hub.BroadcastUpdate(*message)
		}
	}
	WriteJSON(w, status, map[string]any{"guess_id": result.Guess.ID, "photo_id": result.Guess.PhotoID, "score": result.Guess.Score, "timed_out": result.Guess.TimedOut, "created_at": result.Guess.CreatedAt, "duplicate": result.Existing, "server_time": now})
}

// GetChallengeResults returns the leaderboard of a challenge, hiding other
// players' exact points while a hidden-location challenge is still masked.
func (a *GameAPI) GetChallengeResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	photoID := r.PathValue("photoID")
	now := a.clock()
	viewerID := GetUserIDFromContext(r)
	photo, allowed, err := a.groups.CanViewResults(r.Context(), photoID, viewerID, now)
	if err != nil {
		if errors.Is(err, groups.ErrNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "Challenge not found")
		} else if errors.Is(err, groups.ErrForbidden) {
			WriteError(w, http.StatusForbidden, "forbidden", "Results are not available")
		} else {
			WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load results")
		}
		return
	}
	if !allowed {
		WriteError(w, http.StatusForbidden, "results_not_available", "Results are not available yet")
		return
	}
	guesses, err := a.groups.GuessesForPhoto(r.Context(), photoID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load results")
		return
	}
	// The results page shows the weekly Elo change only: the same all-group
	// progression the weekly ladders use, driven by the weekly update factor.
	eloDeltas, err := a.groups.WeeklyChallengeEloDeltas(r.Context(), photoID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load results")
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
		var eloDelta int
		if eloDeltas != nil {
			eloDelta = eloDeltas[guess.UserID]
		}
		item := resultsGuess{
			ID:        guess.ID,
			PhotoID:   guess.PhotoID,
			UserID:    guess.UserID,
			GroupID:   guess.GroupID,
			Score:     guess.Score,
			TimedOut:  guess.TimedOut,
			CreatedAt: guess.CreatedAt,
			Username:  guess.Username,
			Avatar:    guess.Avatar,
			EloDelta:  eloDelta,
		}
		if !hidden || guess.UserID == viewerID {
			if !guess.TimedOut {
				item.Lat = &guess.Lat
				item.Long = &guess.Long
				item.Distance = &guess.Distance
			}
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
	WriteJSON(w, http.StatusOK, response)
}

// resultsGuess is the results payload for a single guess. Lat, long, and
// distance are optional and omitted while a hidden-location challenge keeps
// other players' guessed points private. TimedOut marks a player who let the
// guess window expire (score 0, no location). EloDelta is the signed change
// in the player's weekly Elo rating caused by this challenge.
type resultsGuess struct {
	ID        string    `json:"id"`
	PhotoID   string    `json:"photo_id"`
	UserID    string    `json:"user_id"`
	GroupID   string    `json:"group_id"`
	Lat       *float64  `json:"lat,omitempty"`
	Long      *float64  `json:"long,omitempty"`
	Score     int       `json:"score"`
	Distance  *float64  `json:"distance,omitempty"`
	TimedOut  bool      `json:"timed_out"`
	CreatedAt time.Time `json:"created_at"`
	Username  string    `json:"username"`
	Avatar    string    `json:"avatar"`
	EloDelta  int       `json:"elo_delta"`
}
