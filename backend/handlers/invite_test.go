package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/pashagolub/pgxmock/v4"
)

// inviteColumns is the canonical group_invites result shape used by the mock
// expectations below.
var inviteColumns = []string{"id", "group_id", "creator_user_id", "token_hash", "created_at", "expires_at", "revoked_at"}

func TestCreateInviteReturnsTokenOnce(t *testing.T) {
	mock := newMockPool(t)
	gameAPI := newGameAPI(t, mock)
	groupID := "00000000-0000-0000-0000-000000000001"

	// Membership gate.
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	// Cap serialization locks + counts.
	mock.ExpectBegin()
	mock.ExpectExec("SELECT 1 FROM groups").WithArgs(groupID).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectExec("SELECT 1 FROM users").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM group_invites WHERE group_id").WithArgs(groupID).WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM group_invites WHERE creator_user_id").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec("INSERT INTO group_invites").WithArgs(pgxmock.AnyArg(), groupID, "user-1", pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	rec := httptest.NewRecorder()
	gameAPI.CreateInvite(rec, requestWithUser(http.MethodPost, "/", `{"group_id":"`+groupID+`"}`, "user-1"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invite status = %d (%s)", rec.Code, rec.Body.String())
	}
	var body struct {
		ID        string `json:"id"`
		GroupID   string `json:"group_id"`
		Token     string `json:"token"`
		InviteURL string `json:"invite_url"`
	}
	if err := decodeJSONBody(rec, &body); err != nil {
		t.Fatalf("decode create invite: %v", err)
	}
	if body.Token == "" || body.ID == "" || body.GroupID != groupID {
		t.Fatalf("create invite body = %+v", body)
	}
	if !strings.Contains(body.InviteURL, "/group/join#invite=") {
		t.Fatalf("invite_url = %q, want fragment-based join URL", body.InviteURL)
	}
	if strings.Contains(rec.Body.String(), "token_hash") {
		t.Fatalf("create invite leaked token_hash: %s", rec.Body.String())
	}
}

func TestCreateInviteRejectsNonMember(t *testing.T) {
	mock := newMockPool(t)
	gameAPI := newGameAPI(t, mock)
	groupID := "00000000-0000-0000-0000-000000000001"

	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	rec := httptest.NewRecorder()
	gameAPI.CreateInvite(rec, requestWithUser(http.MethodPost, "/", `{"group_id":"`+groupID+`"}`, "user-1"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-member create invite status = %d, want 403", rec.Code)
	}
}

func TestListInvitesNeverLeaksToken(t *testing.T) {
	mock := newMockPool(t)
	gameAPI := newGameAPI(t, mock)
	groupID := "00000000-0000-0000-0000-000000000001"
	now := time.Now().UTC()

	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery("SELECT id, group_id, creator_user_id, token_hash, created_at, expires_at, revoked_at FROM group_invites WHERE group_id").WithArgs(groupID).
		WillReturnRows(pgxmock.NewRows(inviteColumns).AddRow("invite-1", groupID, "user-1", "hash-1", now, now.Add(time.Hour), nil))

	req := requestWithUser(http.MethodGet, "/?group_id="+groupID, "", "user-1")
	rec := httptest.NewRecorder()
	gameAPI.ListInvites(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list invites status = %d", rec.Code)
	}
	var items []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("list length = %d, want 1", len(items))
	}
	for key := range items[0] {
		if key == "token" || key == "token_hash" {
			t.Fatalf("list leaked %q: %s", key, rec.Body.String())
		}
	}
}

func TestRevokeInviteRequiresMembership(t *testing.T) {
	mock := newMockPool(t)
	gameAPI := newGameAPI(t, mock)
	now := time.Now().UTC()
	groupID := "00000000-0000-0000-0000-000000000001"
	inviteID := "00000000-0000-0000-0000-0000000000aa"

	// Invite lookup by id.
	mock.ExpectQuery("SELECT id, group_id, creator_user_id, token_hash, created_at, expires_at, revoked_at FROM group_invites WHERE id").WithArgs(inviteID).
		WillReturnRows(pgxmock.NewRows(inviteColumns).AddRow(inviteID, groupID, "user-2", "hash", now, now.Add(time.Hour), nil))
	// Non-member: forbidden.
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))

	req := requestWithUser(http.MethodDelete, "/", "", "user-1")
	req.SetPathValue("inviteID", inviteID)
	rec := httptest.NewRecorder()
	gameAPI.RevokeInvite(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-member revoke status = %d, want 403", rec.Code)
	}
}

func TestRevokeInviteSuccessAndMissing(t *testing.T) {
	mock := newMockPool(t)
	gameAPI := newGameAPI(t, mock)
	now := time.Now().UTC()
	groupID := "00000000-0000-0000-0000-000000000001"
	inviteID := "00000000-0000-0000-0000-0000000000aa"

	mock.ExpectQuery("SELECT id, group_id, creator_user_id, token_hash, created_at, expires_at, revoked_at FROM group_invites WHERE id").WithArgs(inviteID).
		WillReturnRows(pgxmock.NewRows(inviteColumns).AddRow(inviteID, groupID, "user-1", "hash", now, now.Add(time.Hour), nil))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(groupID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("UPDATE group_invites SET revoked_at = now\\(\\)").WithArgs(inviteID, groupID).WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	req := requestWithUser(http.MethodDelete, "/", "", "user-1")
	req.SetPathValue("inviteID", inviteID)
	rec := httptest.NewRecorder()
	gameAPI.RevokeInvite(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revoke status = %d, want 204", rec.Code)
	}

	// Missing invite: 404.
	mock.ExpectQuery("SELECT id, group_id, creator_user_id, token_hash, created_at, expires_at, revoked_at FROM group_invites WHERE id").WithArgs("00000000-0000-0000-0000-0000000000bb").
		WillReturnRows(pgxmock.NewRows(inviteColumns))
	req = requestWithUser(http.MethodDelete, "/", "", "user-1")
	req.SetPathValue("inviteID", "00000000-0000-0000-0000-0000000000bb")
	rec = httptest.NewRecorder()
	gameAPI.RevokeInvite(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing revoke status = %d, want 404", rec.Code)
	}
}

func TestJoinGroupRejectsLegacyCode(t *testing.T) {
	mock := newMockPool(t)
	gameAPI := newGameAPI(t, mock)
	// No DB interaction: the missing invite token short-circuits to 410.
	rec := httptest.NewRecorder()
	gameAPI.JoinGroup(rec, requestWithUser(http.MethodPost, "/", `{}`, "user-1"))
	if rec.Code != http.StatusGone {
		t.Fatalf("legacy join status = %d, want 410", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "legacy_group_code_disabled") {
		t.Fatalf("legacy join body = %q", rec.Body.String())
	}
}

func TestJoinGroupExpiredOrRevokedInvite(t *testing.T) {
	mock := newMockPool(t)
	gameAPI := newGameAPI(t, mock)
	now := time.Now().UTC()

	for name, rows := range map[string]*pgxmock.Rows{
		"expired": pgxmock.NewRows(inviteColumns).AddRow("invite-1", "00000000-0000-0000-0000-000000000001", "user-2", "hash", now.Add(-2*time.Hour), now.Add(-time.Hour), nil),
		"revoked": pgxmock.NewRows(inviteColumns).AddRow("invite-2", "00000000-0000-0000-0000-000000000001", "user-2", "hash", now, now.Add(time.Hour), &now),
	} {
		t.Run(name, func(t *testing.T) {
			mock.ExpectQuery("SELECT id, group_id, creator_user_id, token_hash, created_at, expires_at, revoked_at FROM group_invites WHERE token_hash").
				WithArgs(pgxmock.AnyArg()).WillReturnRows(rows)
			rec := httptest.NewRecorder()
			gameAPI.JoinGroup(rec, requestWithUser(http.MethodPost, "/", `{"invite_token":"abc"}`, "user-1"))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("join %s status = %d, want 404", name, rec.Code)
			}
		})
	}
}

func TestCreateInviteGroupCodeHiddenFromGroupWire(t *testing.T) {
	// Group.Code must never serialize (json:"-") so the legacy join code does
	// not leak through any group response.
	group := models.Group{ID: "g1", Name: "Paris", Code: "ABC123"}
	data, err := json.Marshal(group)
	if err != nil {
		t.Fatalf("marshal group: %v", err)
	}
	if strings.Contains(string(data), "ABC123") || strings.Contains(string(data), "code") {
		t.Fatalf("group wire leaked code: %s", data)
	}
}
