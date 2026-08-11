package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"geoguessme/handlers"
	authsvc "geoguessme/internal/auth"
	"geoguessme/internal/progression"
	"geoguessme/internal/validation"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

// UpdateProfile changes the authenticated user's public account fields. A GET
// request returns the current profile instead.
func (a *AuthAPI) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		a.GetProfile(w, r)
		return
	}
	if r.Method != http.MethodPatch {
		handlers.MethodNotAllowed(w)
		return
	}
	var req profileUpdateRequest
	if !handlers.DecodeJSON(w, r, &req) {
		return
	}
	userID := handlers.GetUserIDFromContext(r)
	user, err := a.repos.GetUserByID(r.Context(), userID)
	if err != nil || user == nil || !authsvc.CheckPasswordHash(req.CurrentPassword, user.Password) {
		handlers.WriteError(w, http.StatusUnauthorized, "authentication_failed", "Current password is incorrect")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	if err := validation.ValidateUsername(req.Username); err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_username", err.Error())
		return
	}
	if req.Email != "" {
		if err := validation.ValidateEmail(req.Email); err != nil {
			handlers.WriteError(w, http.StatusBadRequest, "invalid_email", err.Error())
			return
		}
	}
	if !isAvailableAvatar(req.Avatar) {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_avatar", "Choose one of the available avatars")
		return
	}
	if other, lookupErr := a.repos.GetUserByUsername(r.Context(), req.Username); lookupErr != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to update profile")
		return
	} else if other != nil && other.ID != userID {
		handlers.WriteError(w, http.StatusConflict, "username_taken", "Username is already in use")
		return
	}
	// A submitted email becomes a pending claim, not a replacement verified
	// address: the verified recovery address stays active until the new claim
	// is promoted by a successful verification. Pending claims never collide
	// with other accounts (verified-email uniqueness is enforced atomically at
	// promotion), so no email availability check is performed here.
	updated, err := a.repos.UpdateProfile(r.Context(), userID, req.Username, req.Email, req.Avatar)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			handlers.WriteError(w, http.StatusConflict, "profile_update_failed", "Unable to update profile")
			return
		}
		slog.Error("profile update failed", "error", err, "user_id", userID)
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to update profile")
		return
	}
	if updated == nil {
		handlers.WriteError(w, http.StatusConflict, "profile_update_failed", "Unable to update profile")
		return
	}
	handlers.WriteJSON(w, http.StatusOK, userResponse(updated))
}

// GetProfile returns the authenticated player's own profile including global
// ranking data.
func (a *AuthAPI) GetProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		handlers.MethodNotAllowed(w)
		return
	}
	user, err := a.repos.GetUserByID(r.Context(), handlers.GetUserIDFromContext(r))
	if err != nil || user == nil {
		handlers.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	stats, err := a.repos.GetUserScoreStats(r.Context(), user.ID)
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	globalRank, err := a.repos.GetGlobalRank(r.Context(), user.ID)
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	globalAverageRank, err := a.repos.Groups.GlobalAverageRank(r.Context(), user.ID)
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	globalElo, err := a.repos.Groups.GlobalElo(r.Context(), user.ID)
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	handlers.WriteJSON(w, http.StatusOK, ProfileResponse{
		AuthUser:          userResponse(user),
		TotalPoints:       stats.TotalPoints,
		GuessCount:        stats.GuessCount,
		AverageScore:      stats.AverageScore,
		Elo:               globalElo.Elo,
		Rank:              progression.RankForPoints(stats.TotalPoints),
		GlobalRank:        GlobalRank{Rank: globalRank.Rank, TotalPlayers: globalRank.TotalPlayers},
		GlobalAverageRank: GlobalRank{Rank: globalAverageRank.Rank, TotalPlayers: globalAverageRank.TotalPlayers},
		GlobalEloRank:     GlobalRank{Rank: globalElo.Rank, TotalPlayers: globalElo.TotalPlayers},
	})
}

// GetPublicProfile returns another player's progression profile. The player
// must share at least one group with the requester (or be the requester
// themselves); email and account details are never returned.
func (a *AuthAPI) GetPublicProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		handlers.MethodNotAllowed(w)
		return
	}
	targetID := strings.TrimSpace(r.PathValue("userID"))
	viewerID := handlers.GetUserIDFromContext(r)
	user, err := a.repos.GetUserByID(r.Context(), targetID)
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	if user == nil {
		handlers.WriteError(w, http.StatusNotFound, "not_found", "Player not found")
		return
	}
	if targetID != viewerID {
		shared, err := a.repos.Groups.SharesGroup(r.Context(), targetID, viewerID)
		if err != nil {
			handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
			return
		}
		if !shared {
			handlers.WriteError(w, http.StatusForbidden, "forbidden", "You do not share a group with this player")
			return
		}
	}
	stats, err := a.repos.GetUserScoreStats(r.Context(), user.ID)
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	globalRank, err := a.repos.GetGlobalRank(r.Context(), user.ID)
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	globalAverageRank, err := a.repos.Groups.GlobalAverageRank(r.Context(), user.ID)
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	globalElo, err := a.repos.Groups.GlobalElo(r.Context(), user.ID)
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	handlers.WriteJSON(w, http.StatusOK, PublicProfileResponse{
		ID:                user.ID,
		Username:          user.Username,
		Avatar:            user.Avatar,
		TotalPoints:       stats.TotalPoints,
		GuessCount:        stats.GuessCount,
		AverageScore:      stats.AverageScore,
		Elo:               globalElo.Elo,
		Rank:              progression.RankForPoints(stats.TotalPoints),
		GlobalRank:        GlobalRank{Rank: globalRank.Rank, TotalPlayers: globalRank.TotalPlayers},
		GlobalAverageRank: GlobalRank{Rank: globalAverageRank.Rank, TotalPlayers: globalAverageRank.TotalPlayers},
		GlobalEloRank:     GlobalRank{Rank: globalElo.Rank, TotalPlayers: globalElo.TotalPlayers},
	})
}

// ChangePassword updates the password after confirming the current one and
// clears the refresh cookie (the change revokes every session).
func (a *AuthAPI) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		handlers.MethodNotAllowed(w)
		return
	}
	var req passwordChangeRequest
	if !handlers.DecodeJSON(w, r, &req) {
		return
	}
	userID := handlers.GetUserIDFromContext(r)
	user, err := a.repos.GetUserByID(r.Context(), userID)
	if err != nil || user == nil || !authsvc.CheckPasswordHash(req.CurrentPassword, user.Password) {
		handlers.WriteError(w, http.StatusUnauthorized, "authentication_failed", "Current password is incorrect")
		return
	}
	if err := validation.ValidatePassword(req.NewPassword); err != nil {
		handlers.WriteError(w, http.StatusBadRequest, "invalid_password", err.Error())
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), a.configuredCost())
	if err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to change password")
		return
	}
	if err := a.repos.ChangePassword(r.Context(), userID, string(hash)); err != nil {
		handlers.WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to change password")
		return
	}
	// The change bumped the auth version and revoked every session; sockets
	// opened under the old version must close.
	a.kickDisconnectUser(userID)
	a.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}
