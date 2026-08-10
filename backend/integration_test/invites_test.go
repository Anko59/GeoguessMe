package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInviteTokenContract(t *testing.T) {
	alice := signup(t, unique("invitea"), unique("invitea")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("inviteb"), unique("inviteb")+"@example.test", "StrongPassword123")
	carol := signup(t, unique("invitec"), unique("invitec")+"@example.test", "StrongPassword123")

	groupID, inviteToken := createGroup(t, alice.access, "Invite Contract Group")

	// The raw token is returned exactly once at creation, inside a fragment
	// invite URL.
	inviteURL := createInviteAndAssert(t, alice.access, groupID)
	require.True(t, strings.HasPrefix(inviteURL, "/group/join#invite="), "invite_url = %q", inviteURL)
	fragmentToken := strings.TrimPrefix(inviteURL, "/group/join#invite=")
	require.NotEmpty(t, fragmentToken)

	// The token returned by createGroup must be distinct from the invite-URL
	// token (each creation mints a fresh bearer value).
	require.NotEqual(t, inviteToken, fragmentToken)

	// Multi-recipient: two different users join with the same token.
	joinGroup(t, bob.access, inviteToken)
	joinGroup(t, carol.access, inviteToken)

	// Joining again is idempotent for an existing member.
	joinGroup(t, bob.access, inviteToken)
}

func TestInviteListOmitsToken(t *testing.T) {
	alice := signup(t, unique("inviteal"), unique("inviteal")+"@example.test", "StrongPassword123")
	groupID, inviteToken := createGroup(t, alice.access, "Invite List Group")

	resp, data := doJSON(t, http.MethodGet, "/api/v1/group/invites?group_id="+groupID, nil, alice.access, nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "list invites: %s", data)
	var items []map[string]any
	require.NoError(t, json.Unmarshal(data, &items))
	require.Len(t, items, 1, "one invite expected")
	body := string(data)
	require.NotContains(t, body, inviteToken, "list response leaked the bearer token")
	require.NotContains(t, body, "token_hash", "list response leaked token_hash")
}

func TestInviteRevokeInvalidatesToken(t *testing.T) {
	alice := signup(t, unique("inviterev"), unique("inviterev")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("inviterevb"), unique("inviterevb")+"@example.test", "StrongPassword123")
	groupID, inviteToken := createGroup(t, alice.access, "Invite Revoke Group")

	// Locate the invite id through the list endpoint.
	resp, data := doJSON(t, http.MethodGet, "/api/v1/group/invites?group_id="+groupID, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var items []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(data, &items))
	require.Len(t, items, 1)
	inviteID := items[0].ID

	// Revoke it.
	resp, _ = doJSON(t, http.MethodDelete, "/api/v1/group/invites/"+inviteID, nil, alice.access, nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	// A revoked token can no longer join.
	resp, data = doJSON(t, http.MethodPost, "/api/v1/group/join", map[string]string{"invite_token": inviteToken}, bob.access, nil)
	require.Equalf(t, http.StatusNotFound, resp.StatusCode, "join with revoked token: %s", data)

	// The public preview also refuses the revoked token with a generic 404.
	resp, data = doJSON(t, http.MethodPost, "/api/v1/group/invites/preview", map[string]string{"invite_token": inviteToken}, "", nil)
	require.Equalf(t, http.StatusNotFound, resp.StatusCode, "preview revoked token: %s", data)
}

func TestInvitePreviewIsNonSensitiveAndPublic(t *testing.T) {
	alice := signup(t, unique("inviteprv"), unique("inviteprv")+"@example.test", "StrongPassword123")
	groupID, inviteToken := createGroup(t, alice.access, "Invite Preview Group")
	bob := signup(t, unique("inviteprvb"), unique("inviteprvb")+"@example.test", "StrongPassword123")
	joinGroup(t, bob.access, inviteToken)

	// Public: no auth token sent.
	resp, data := doJSON(t, http.MethodPost, "/api/v1/group/invites/preview", map[string]string{"invite_token": inviteToken}, "", nil)
	require.Equalf(t, http.StatusOK, resp.StatusCode, "preview: %s", data)
	var preview struct {
		GroupName   string `json:"group_name"`
		MemberCount int    `json:"member_count"`
	}
	require.NoError(t, json.Unmarshal(data, &preview))
	require.Equal(t, "Invite Preview Group", preview.GroupName)
	require.Equal(t, 2, preview.MemberCount)
	// The preview exposes neither the group id nor the creator nor the token.
	require.NotContains(t, string(data), groupID)
	require.NotContains(t, string(data), inviteToken)

	// Unknown tokens are a generic 404 with no leak.
	resp, data = doJSON(t, http.MethodPost, "/api/v1/group/invites/preview", map[string]string{"invite_token": "does-not-exist"}, "", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.Contains(t, string(data), "invite_not_found")
}

func TestInviteExpiryBlocksJoin(t *testing.T) {
	alice := signup(t, unique("inviteexp"), unique("inviteexp")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("inviteexpb"), unique("inviteexpb")+"@example.test", "StrongPassword123")
	groupID, inviteToken := createGroup(t, alice.access, "Invite Expiry Group")

	db := testDB(t)
	// Force the invite to expire immediately.
	tag, err := db.Exec(context.Background(),
		`UPDATE group_invites SET expires_at = now() - interval '1 minute' WHERE group_id = $1 AND revoked_at IS NULL`, groupID)
	require.NoError(t, err)
	require.Equal(t, int64(1), tag.RowsAffected())

	resp, data := doJSON(t, http.MethodPost, "/api/v1/group/join", map[string]string{"invite_token": inviteToken}, bob.access, nil)
	require.Equalf(t, http.StatusNotFound, resp.StatusCode, "join expired token: %s", data)

	resp, data = doJSON(t, http.MethodPost, "/api/v1/group/invites/preview", map[string]string{"invite_token": inviteToken}, "", nil)
	require.Equalf(t, http.StatusNotFound, resp.StatusCode, "preview expired token: %s", data)
}

func TestInviteCreateCapAndLegacyJoinDisabled(t *testing.T) {
	alice := signup(t, unique("invitecap"), unique("invitecap")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("invitecapb"), unique("invitecapb")+"@example.test", "StrongPassword123")
	groupID, inviteToken := createGroup(t, alice.access, "Invite Cap Group")

	// A group may hold at most five active invites.
	for i := 0; i < 4; i++ {
		resp, data := doJSON(t, http.MethodPost, "/api/v1/group/invites", map[string]string{"group_id": groupID}, alice.access, nil)
		require.Equalf(t, http.StatusCreated, resp.StatusCode, "invite %d: %s", i, data)
	}
	resp, data := doJSON(t, http.MethodPost, "/api/v1/group/invites", map[string]string{"group_id": groupID}, alice.access, nil)
	require.Equalf(t, http.StatusConflict, resp.StatusCode, "sixth invite: %s", data)
	require.Contains(t, string(data), "too_many_active_invites")

	// Legacy typed-code join is disabled with a 410.
	resp, data = doJSON(t, http.MethodPost, "/api/v1/group/join", map[string]string{"code": "ABC123"}, bob.access, nil)
	require.Equalf(t, http.StatusGone, resp.StatusCode, "legacy join: %s", data)
	require.Contains(t, string(data), "legacy_group_code_disabled")

	// Revoking an invite frees a slot for the active cap.
	resp, data = doJSON(t, http.MethodGet, "/api/v1/group/invites?group_id="+groupID, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var items []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(data, &items))
	require.Len(t, items, 5)
	// ListInvitesByGroup returns newest invites first (ORDER BY created_at
	// DESC), so the last item is the oldest invite: the one issued by
	// createGroup above. Revoking it lets us prove that the token we hold
	// (inviteToken) becomes invalid and that the revoked slot frees up.
	revokedID := items[len(items)-1].ID
	_, _ = doJSON(t, http.MethodDelete, "/api/v1/group/invites/"+revokedID, nil, alice.access, nil)
	resp, data = doJSON(t, http.MethodPost, "/api/v1/group/invites", map[string]string{"group_id": groupID}, alice.access, nil)
	require.Equalf(t, http.StatusCreated, resp.StatusCode, "invite after revoke frees slot: %s", data)

	// The stored token is a SHA-256 hash, never the raw bearer value.
	db := testDB(t)
	var tokenHash string
	err := db.QueryRow(context.Background(),
		`SELECT token_hash FROM group_invites WHERE group_id = $1 AND revoked_at IS NULL LIMIT 1`, groupID).Scan(&tokenHash)
	require.NoError(t, err)
	require.NotEqual(t, inviteToken, tokenHash, "raw bearer token must never be stored")

	// Sanity: preview works again for a fresh invite (multi-recipient token).
	resp, data = doJSON(t, http.MethodPost, "/api/v1/group/invites/preview", map[string]string{"invite_token": inviteToken}, "", nil)
	require.Equalf(t, http.StatusNotFound, resp.StatusCode, "revoked token must stay invalid: %s", data)
}

// createInviteAndAssert creates an invite and returns its invite_url.
func createInviteAndAssert(t *testing.T, bearer, groupID string) string {
	t.Helper()
	resp, data := doJSON(t, http.MethodPost, "/api/v1/group/invites", map[string]string{"group_id": groupID}, bearer, nil)
	require.Equalf(t, http.StatusCreated, resp.StatusCode, "create invite: %s", data)
	var result struct {
		ID        string    `json:"id"`
		Token     string    `json:"token"`
		InviteURL string    `json:"invite_url"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	require.NoError(t, json.Unmarshal(data, &result))
	require.NotEmpty(t, result.Token)
	require.Contains(t, string(data), result.Token, "raw token must appear exactly once in the create response")
	// Seven-day expiry, with a small clock allowance.
	require.WithinDuration(t, time.Now().UTC().Add(7*24*time.Hour), result.ExpiresAt, time.Minute, "expires_at must be ~7 days from creation")
	return result.InviteURL
}
