package integration_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNonMemberForbidden(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	outsider := signup(t, unique("out"), unique("out")+"@example.test", "StrongPassword123")
	groupID, _ := createGroup(t, alice.access, "Private Group")

	resp, _ := doJSON(t, http.MethodGet, "/api/v1/group/leaderboard?group_id="+groupID, nil, outsider.access, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/group/messages?group_id="+groupID, nil, outsider.access, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp, _ = doJSON(t, http.MethodPost, "/api/v1/ws/ticket?group_id="+groupID, nil, outsider.access, nil)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestCrossGroupIsolation(t *testing.T) {
	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	carol := signup(t, unique("carol"), unique("carol")+"@example.test", "StrongPassword123")

	groupA, codeA := createGroup(t, alice.access, "Group A")
	groupB, codeB := createGroup(t, carol.access, "Group B")
	joinGroup(t, bob.access, codeA)
	joinGroup(t, bob.access, codeB)

	photoA := uploadPhoto(t, alice.access, groupA)
	waitUntilViewCloses(t, bob.access, photoA)
	guess(t, bob.access, photoA, 12, 34)

	// Group A leaderboard records the guess; Group B does not.
	respA, dataA := doJSON(t, http.MethodGet, "/api/v1/group/leaderboard?group_id="+groupA, nil, alice.access, nil)
	require.Equal(t, http.StatusOK, respA.StatusCode)
	require.Contains(t, string(dataA), "bob")

	respB, dataB := doJSON(t, http.MethodGet, "/api/v1/group/leaderboard?group_id="+groupB, nil, carol.access, nil)
	require.Equal(t, http.StatusOK, respB.StatusCode)
	require.NotContains(t, string(dataB), `"score":`)
	_ = dataB
}
