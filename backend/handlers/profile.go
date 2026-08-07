package handlers

import (
	"net/http"
	"strings"

	"geoguessme/internal/auth"
	"geoguessme/internal/progression"
	"geoguessme/internal/repository"
	"geoguessme/internal/validation"

	"golang.org/x/crypto/bcrypt"
)

type profileUpdateRequest struct {
	Username        string `json:"username"`
	Email           string `json:"email"`
	Avatar          string `json:"avatar"`
	CurrentPassword string `json:"current_password"`
}

type passwordChangeRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func UpdateProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		GetProfile(w, r)
		return
	}
	if r.Method != http.MethodPatch {
		methodNotAllowed(w)
		return
	}
	var req profileUpdateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	userID := GetUserIDFromContext(r)
	user, err := repository.GetUserByID(r.Context(), userID)
	if err != nil || user == nil || !auth.CheckPasswordHash(req.CurrentPassword, user.Password) {
		writeError(w, http.StatusUnauthorized, "authentication_failed", "Current password is incorrect")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.TrimSpace(req.Email)
	if err := validation.ValidateUsername(req.Username); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_username", err.Error())
		return
	}
	if err := validation.ValidateEmail(req.Email); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_email", err.Error())
		return
	}
	if !isAvailableAvatar(req.Avatar) {
		writeError(w, http.StatusBadRequest, "invalid_avatar", "Choose one of the available avatars")
		return
	}
	if other, lookupErr := repository.GetUserByUsernameContext(r.Context(), req.Username); lookupErr != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to update profile")
		return
	} else if other != nil && other.ID != userID {
		writeError(w, http.StatusConflict, "username_taken", "Username is already in use")
		return
	}
	if other, lookupErr := repository.GetUserByEmailContext(r.Context(), req.Email); lookupErr != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to update profile")
		return
	} else if other != nil && other.ID != userID {
		writeError(w, http.StatusConflict, "email_taken", "Email is already in use")
		return
	}
	updated, err := repository.UpdateProfile(r.Context(), userID, req.Username, req.Email, req.Avatar)
	if err != nil || updated == nil {
		writeError(w, http.StatusConflict, "profile_update_failed", "Unable to update profile")
		return
	}
	writeJSON(w, http.StatusOK, userResponse(updated))
}

// GlobalRank is the player's position among every player who has guessed at
// least once, ordered by lifetime points. Rank is zero while the player has no
// guesses of their own (they are not part of the ranked population).
type GlobalRank struct {
	Rank         int `json:"rank"`
	TotalPlayers int `json:"total_players"`
}

// PublicProfileResponse is the profile of another player. It deliberately
// excludes email and account details; it contains only the identity and
// progression data already visible inside a shared group.
type PublicProfileResponse struct {
	ID                string           `json:"id"`
	Username          string           `json:"username"`
	Avatar            string           `json:"avatar"`
	TotalPoints       int              `json:"total_points"`
	GuessCount        int              `json:"guess_count"`
	AverageScore      float64          `json:"average_score"`
	Elo               int              `json:"elo"`
	Rank              progression.Rank `json:"rank"`
	GlobalRank        GlobalRank       `json:"global_rank"`
	GlobalAverageRank GlobalRank       `json:"global_average_rank"`
	GlobalEloRank     GlobalRank       `json:"global_elo_rank"`
}

type ProfileResponse struct {
	AuthUser
	TotalPoints       int              `json:"total_points"`
	GuessCount        int              `json:"guess_count"`
	AverageScore      float64          `json:"average_score"`
	Elo               int              `json:"elo"`
	Rank              progression.Rank `json:"rank"`
	GlobalRank        GlobalRank       `json:"global_rank"`
	GlobalAverageRank GlobalRank       `json:"global_average_rank"`
	GlobalEloRank     GlobalRank       `json:"global_elo_rank"`
}

func GetProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	user, err := repository.GetUserByID(r.Context(), GetUserIDFromContext(r))
	if err != nil || user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}
	stats, err := repository.GetUserScoreStatsContext(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	globalRank, err := repository.GetGlobalRankContext(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	globalAverageRank, err := repository.GetGlobalAverageRankContext(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	globalElo, err := repository.GetGlobalEloContext(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	writeJSON(w, http.StatusOK, ProfileResponse{
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
func GetPublicProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	targetID := strings.TrimSpace(r.PathValue("userID"))
	viewerID := GetUserIDFromContext(r)
	user, err := repository.GetUserByID(r.Context(), targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "not_found", "Player not found")
		return
	}
	if targetID != viewerID {
		shared, err := repository.SharesGroupContext(r.Context(), targetID, viewerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
			return
		}
		if !shared {
			writeError(w, http.StatusForbidden, "forbidden", "You do not share a group with this player")
			return
		}
	}
	stats, err := repository.GetUserScoreStatsContext(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	globalRank, err := repository.GetGlobalRankContext(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	globalAverageRank, err := repository.GetGlobalAverageRankContext(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	globalElo, err := repository.GetGlobalEloContext(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load profile")
		return
	}
	writeJSON(w, http.StatusOK, PublicProfileResponse{
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

func ChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req passwordChangeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	userID := GetUserIDFromContext(r)
	user, err := repository.GetUserByID(r.Context(), userID)
	if err != nil || user == nil || !auth.CheckPasswordHash(req.CurrentPassword, user.Password) {
		writeError(w, http.StatusUnauthorized, "authentication_failed", "Current password is incorrect")
		return
	}
	if err := validation.ValidatePassword(req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_password", err.Error())
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), configuredCost())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to change password")
		return
	}
	if err := repository.ChangePassword(r.Context(), userID, string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to change password")
		return
	}
	clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func isAvailableAvatar(avatar string) bool {
	if IsCustomAvatar(avatar) {
		return true
	}
	for _, candidate := range availableAvatars {
		if avatar == candidate {
			return true
		}
	}
	return false
}
