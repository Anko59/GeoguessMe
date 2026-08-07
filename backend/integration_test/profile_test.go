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
		ID       string `json:"id"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Rank     struct {
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
