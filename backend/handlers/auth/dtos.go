package auth

import (
	"time"

	"geoguessme/internal/models"
	"geoguessme/internal/progression"
)

// SignupRequest is the wire payload for account creation.
type SignupRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest is the wire payload for password login.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthUser is the public account shape returned by auth and profile flows.
type AuthUser struct {
	ID              string     `json:"id"`
	Username        string     `json:"username"`
	Email           string     `json:"email"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	Avatar          string     `json:"avatar"`
}

// AuthResponse is the session start payload.
type AuthResponse struct {
	AccessToken string   `json:"access_token"`
	ExpiresIn   int64    `json:"expires_in"`
	User        AuthUser `json:"user"`
}

// TokenRequest is the payload for email-verification and password-reset
// confirmation endpoints.
type TokenRequest struct {
	Token string `json:"token"`
}

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

// ProfileResponse is the authenticated player's own profile.
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

// userResponse maps a user row onto the wire-shaped AuthUser view model.
func userResponse(user *models.User) AuthUser {
	return AuthUser{ID: user.ID, Username: user.Username, Email: user.Email, EmailVerifiedAt: user.EmailVerifiedAt, Avatar: user.Avatar}
}
