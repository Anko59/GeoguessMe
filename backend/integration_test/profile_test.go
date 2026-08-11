package integration_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicProfileVisibility(t *testing.T) {
	alice := signup(t, unique("ppalice"), unique("ppalice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("ppbob"), unique("ppbob")+"@example.test", "StrongPassword123")
	charlie := signup(t, unique("ppcharlie"), unique("ppcharlie")+"@example.test", "StrongPassword123")

	_, code := createGroup(t, alice.access, "Profile test group")
	joinGroup(t, bob.access, code)

	// A shared group lets bob see alice's progression, without any email.
	resp, data := doJSON(t, http.MethodGet, "/api/v1/user/profile/"+alice.userID, nil, bob.access, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "shared profile: %s", data)
	var profile struct {
		ID         string  `json:"id"`
		Username   string  `json:"username"`
		Email      string  `json:"email"`
		TotalPts   int     `json:"total_points"`
		AvgScore   float64 `json:"average_score"`
		Elo        int     `json:"elo"`
		GlobalRank struct {
			Rank int `json:"rank"`
		} `json:"global_rank"`
		GlobalAvgRank struct {
			Rank int `json:"rank"`
		} `json:"global_average_rank"`
		GlobalEloRank struct {
			Rank int `json:"rank"`
		} `json:"global_elo_rank"`
		Rank struct {
			Level    int    `json:"level"`
			Name     string `json:"name"`
			NextRank *struct {
				Name string `json:"name"`
			} `json:"next_rank"`
		} `json:"rank"`
	}
	require.NoError(t, json.Unmarshal(data, &profile))
	require.Equal(t, alice.userID, profile.ID)
	require.Equal(t, 1, profile.Rank.Level)
	require.Equal(t, "Completely Lost", profile.Rank.Name)
	require.Empty(t, profile.Email, "public profile must not expose email")
	require.NotNil(t, profile.Rank.NextRank)
	// The three global rankings are present; a fresh player is unranked in
	// all of them until they guess a location.
	require.Zero(t, profile.GlobalRank.Rank)
	require.Zero(t, profile.GlobalAvgRank.Rank)
	require.Zero(t, profile.GlobalEloRank.Rank)
	require.Zero(t, profile.Elo)

	// A player who shares no group gets a 403.
	resp, data = doJSON(t, http.MethodGet, "/api/v1/user/profile/"+alice.userID, nil, charlie.access, nil)
	require.Equalf(t, http.StatusForbidden, resp.StatusCode, "stranger profile: %s", data)

	// Viewing yourself always works, through the same endpoint.
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/user/profile/"+alice.userID, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Unknown players are not found.
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/user/profile/does-not-exist", nil, alice.access, nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestVerificationTokenIsBoundToPendingEmail proves a token issued for one
// pending claim cannot promote a replacement claim entered later.
func TestVerificationTokenIsBoundToPendingEmail(t *testing.T) {
	resetRateLimiter(t)
	user := unique("bound-claim")
	firstEmail := user + "-first@example.test"
	secondEmail := user + "-second@example.test"
	const pass = "StrongPassword123"
	session := signup(t, user, firstEmail, pass)
	staleToken := tokensFromMailpitTo(t, "Verify your GeoGuessMe email", "/verify-email", firstEmail, 1)[0]

	resp, data := doJSON(t, http.MethodPatch, "/api/v1/auth/profile", map[string]string{
		"username":         user,
		"email":            secondEmail,
		"avatar":           "avatar.png",
		"current_password": pass,
	}, session.access, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "profile update: %s", data)

	resp, data = doJSON(t, http.MethodPost, "/api/v1/auth/verify", map[string]string{"token": staleToken}, "", nil)
	require.Equalf(t, http.StatusBadRequest, resp.StatusCode, "stale verification: %s", data)
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(data, &envelope))
	require.Equal(t, "invalid_token", envelope.Error.Code)

	resp, data = doJSON(t, http.MethodGet, "/api/v1/auth/profile", nil, session.access, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "profile read: %s", data)
	var profile struct {
		Email        string `json:"email"`
		PendingEmail string `json:"pending_email"`
	}
	require.NoError(t, json.Unmarshal(data, &profile))
	require.Empty(t, profile.Email)
	require.Equal(t, secondEmail, profile.PendingEmail)

	resp, _ = doJSON(t, http.MethodPost, "/api/v1/auth/verify/request", nil, session.access, nil)
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
	currentToken := tokensFromMailpitTo(t, "Verify your GeoGuessMe email", "/verify-email", secondEmail, 1)[0]
	resp, data = doJSON(t, http.MethodPost, "/api/v1/auth/verify", map[string]string{"token": currentToken}, "", nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "current verification: %s", data)
}
