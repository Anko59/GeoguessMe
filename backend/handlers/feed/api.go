package feed

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"geoguessme/handlers"
	"geoguessme/internal/config"
	"geoguessme/internal/repository/feed"
	"geoguessme/internal/storage"
	"geoguessme/internal/validation"

	"github.com/google/uuid"
)

type API struct {
	repo      *feed.Repository
	store     storage.ObjectStore
	deletions handlers.DeletionEnqueuer
	cfg       *config.Config
	clock     func() time.Time
}

func NewAPI(repo *feed.Repository, store storage.ObjectStore, deletions handlers.DeletionEnqueuer, cfg *config.Config, clock func() time.Time) *API {
	return &API{repo: repo, store: store, deletions: deletions, cfg: cfg, clock: clock}
}

func (a *API) Routes(mux *http.ServeMux, protect func(http.HandlerFunc) http.Handler) {
	mux.Handle("/api/v1/feed", protect(a.List))
	mux.Handle("/api/v1/feed/leaderboard", protect(a.Leaderboard))
	mux.Handle("/api/v1/feed/leaderboard/{profileID}", protect(a.ProfileLeaderboard))
	mux.Handle("/api/v1/feed/challenges", protect(a.Upload))
	mux.Handle("/api/v1/feed/challenges/{id}", protect(a.Post))
	mux.Handle("/api/v1/feed/challenges/{id}/media", protect(a.Media))
	mux.Handle("/api/v1/feed/challenges/{id}/play", protect(a.Play))
	mux.Handle("/api/v1/feed/challenges/{id}/accept", protect(a.AcceptTimed))
	mux.Handle("/api/v1/feed/challenges/{id}/timed-media", protect(a.TimedMedia))
	mux.Handle("/api/v1/feed/challenges/{id}/media-delivered", protect(a.MediaDelivered))
	mux.Handle("/api/v1/feed/challenges/{id}/timed-guess", protect(a.TimedGuess))
	mux.Handle("/api/v1/feed/challenges/{id}/timed-timeout", protect(a.TimedTimeout))
	mux.Handle("/api/v1/feed/challenges/{id}/timed-results", protect(a.TimedResults))
	mux.Handle("/api/v1/feed/challenges/{id}/guess", protect(a.Guess))
	mux.Handle("/api/v1/feed/challenges/{id}/results", protect(a.Results))
	mux.Handle("/api/v1/feed/challenges/{id}/reaction", protect(a.Reaction))
	mux.Handle("/api/v1/feed/challenges/{id}/comments", protect(a.Comments))
	mux.Handle("/api/v1/feed/challenges/{id}/comments/{commentID}", protect(a.DeleteComment))
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, feed.ErrNotFound):
		handlers.WriteError(w, 404, "not_found", "Public challenge or comment not found")
	case errors.Is(err, feed.ErrProfileNotFound):
		handlers.WriteError(w, 404, "not_found", "Player not found")
	case errors.Is(err, feed.ErrForbidden):
		handlers.WriteError(w, 403, "forbidden", "You are not allowed to perform this feed action")
	case errors.Is(err, feed.ErrOwnChallenge):
		handlers.WriteError(w, 403, "forbidden", "You cannot guess your own challenge")
	case errors.Is(err, feed.ErrViewNotFinished):
		handlers.WriteError(w, 409, "viewing_window_open", "Wait until the viewing window ends before guessing")
	case errors.Is(err, feed.ErrGuessTimeExpired):
		handlers.WriteError(w, 410, "guess_time_expired", "You did not guess in time")
	case errors.Is(err, feed.ErrMediaExpired):
		handlers.WriteError(w, 403, "media_expired", "The viewing window has expired")
	case errors.Is(err, feed.ErrInvalidCoordinate):
		handlers.WriteError(w, 400, "invalid_coordinates", "Choose a valid location")
	default:
		slog.Error("public feed request failed", "error", err)
		handlers.WriteError(w, 500, "internal_error", "Unable to complete this feed request")
	}
}

func leaderboardPagination(w http.ResponseWriter, r *http.Request) (feed.LeaderboardCursor, int, bool) {
	cursor, err := feed.ParseLeaderboardCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		handlers.WriteError(w, 400, "invalid_cursor", "Invalid leaderboard cursor")
		return cursor, 0, false
	}
	limit := 20
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 50 {
			handlers.WriteError(w, 400, "invalid_limit", "Limit must be between 1 and 50")
			return cursor, 0, false
		}
	}
	return cursor, limit, true
}

func validID(w http.ResponseWriter, r *http.Request) bool {
	if _, err := uuid.Parse(r.PathValue("id")); err != nil {
		handlers.WriteError(w, 400, "invalid_id", "A valid challenge ID is required")
		return false
	}
	return true
}

func pagination(w http.ResponseWriter, r *http.Request) (feed.Cursor, int, bool) {
	cursor, err := feed.ParseCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		handlers.WriteError(w, 400, "invalid_cursor", "Invalid page cursor")
		return cursor, 0, false
	}
	limit := 20
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 50 {
			handlers.WriteError(w, 400, "invalid_limit", "Limit must be between 1 and 50")
			return cursor, 0, false
		}
	}
	return cursor, limit, true
}

func (a *API) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		handlers.MethodNotAllowed(w)
		return
	}
	cursor, limit, ok := pagination(w, r)
	if !ok {
		return
	}
	page, err := a.repo.List(r.Context(), handlers.GetUserIDFromContext(r), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	handlers.WriteJSON(w, 200, page)
}

func (a *API) Leaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		handlers.MethodNotAllowed(w)
		return
	}
	cursor, limit, ok := leaderboardPagination(w, r)
	if !ok {
		return
	}
	page, err := a.repo.Leaderboard(r.Context(), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	handlers.WriteJSON(w, 200, page)
}

// ProfileLeaderboard returns the players who best guessed the requested
// profile owner's visible feed challenges. It keeps profile visibility aligned
// with GET /user/profile/{userID} while leaving the legacy global leaderboard
// available for compatibility.
func (a *API) ProfileLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		handlers.MethodNotAllowed(w)
		return
	}
	profileID := strings.TrimSpace(r.PathValue("profileID"))
	if _, err := uuid.Parse(profileID); err != nil {
		handlers.WriteError(w, 400, "invalid_id", "A valid profile ID is required")
		return
	}
	cursor, limit, ok := leaderboardPagination(w, r)
	if !ok {
		return
	}
	page, err := a.repo.ProfileLeaderboard(r.Context(), profileID, handlers.GetUserIDFromContext(r), cursor, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	handlers.WriteJSON(w, 200, page)
}

func (a *API) Post(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		handlers.MethodNotAllowed(w)
		return
	}
	if !validID(w, r) {
		return
	}
	id, user := r.PathValue("id"), handlers.GetUserIDFromContext(r)
	if r.Method == http.MethodDelete {
		if err := a.repo.Delete(r.Context(), id, user); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(204)
		return
	}
	post, err := a.repo.Get(r.Context(), id, user)
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	handlers.WriteJSON(w, 200, post)
}

func (a *API) Guess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		handlers.MethodNotAllowed(w)
		return
	}
	if !validID(w, r) {
		return
	}
	id, user := r.PathValue("id"), handlers.GetUserIDFromContext(r)
	if r.Method == http.MethodGet {
		result, err := a.repo.Result(r.Context(), id, user)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		handlers.WriteJSON(w, 200, result)
		return
	}
	var req struct {
		Lat  *float64 `json:"lat"`
		Long *float64 `json:"long"`
	}
	if !handlers.DecodeJSON(w, r, &req) {
		return
	}
	if req.Lat == nil || req.Long == nil || validation.ValidateCoordinates(*req.Lat, *req.Long) != nil {
		handlers.WriteError(w, 400, "invalid_coordinates", "Choose a valid location")
		return
	}
	result, err := a.repo.Guess(r.Context(), id, user, *req.Lat, *req.Long)
	if err != nil {
		writeError(w, err)
		return
	}
	handlers.WriteJSON(w, 200, result)
}

func (a *API) Results(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		handlers.MethodNotAllowed(w)
		return
	}
	if !validID(w, r) {
		return
	}
	results, err := a.repo.Results(r.Context(), r.PathValue("id"), handlers.GetUserIDFromContext(r))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	handlers.WriteJSON(w, 200, results)
}

func (a *API) Reaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		handlers.MethodNotAllowed(w)
		return
	}
	if !validID(w, r) {
		return
	}
	if err := a.repo.React(r.Context(), r.PathValue("id"), handlers.GetUserIDFromContext(r), r.Method == http.MethodPut); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(204)
}

func (a *API) Comments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	if !validID(w, r) {
		return
	}
	id, user := r.PathValue("id"), handlers.GetUserIDFromContext(r)
	if r.Method == http.MethodGet {
		cursor, limit, ok := pagination(w, r)
		if !ok {
			return
		}
		page, err := a.repo.Comments(r.Context(), id, user, cursor, limit)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Cache-Control", "private, no-store")
		handlers.WriteJSON(w, 200, page)
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if !handlers.DecodeJSON(w, r, &req) {
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if !utf8.ValidString(req.Content) || utf8.RuneCountInString(req.Content) < 1 || utf8.RuneCountInString(req.Content) > 1000 {
		handlers.WriteError(w, 400, "invalid_comment", "Comments must contain 1 to 1000 characters")
		return
	}
	comment, err := a.repo.Comment(r.Context(), id, user, req.Content)
	if err != nil {
		writeError(w, err)
		return
	}
	handlers.WriteJSON(w, 201, comment)
}

func (a *API) DeleteComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		handlers.MethodNotAllowed(w)
		return
	}
	if !validID(w, r) {
		return
	}
	if _, err := uuid.Parse(r.PathValue("commentID")); err != nil {
		handlers.WriteError(w, 400, "invalid_id", "A valid comment ID is required")
		return
	}
	if err := a.repo.DeleteComment(r.Context(), r.PathValue("id"), r.PathValue("commentID"), handlers.GetUserIDFromContext(r)); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(204)
}
