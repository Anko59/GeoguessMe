package handlers

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"geoguessme/internal/auth"
	"geoguessme/internal/models"
	"geoguessme/internal/repository/groups"

	"github.com/google/uuid"
)

// Invite lifecycle constants (F-06). Invites are bearer tokens that expire
// after seven days, a group may hold at most five active invites, and each
// user may create at most ten invites per day.
const (
	inviteTTL                = 7 * 24 * time.Hour
	maxActiveInvitesPerGroup = 5
	maxInvitesPerUserPerDay  = 10
)

// validInviteToken accepts only the canonical 32-byte RawURL base64 form
// emitted by GenerateOpaqueToken. Rejecting malformed credentials before
// hashing avoids unnecessary database work and keeps bearer parsing exact.
func validInviteToken(token string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(decoded) == 32 && base64.RawURLEncoding.EncodeToString(decoded) == token
}

type CreateInviteRequest struct {
	GroupID string `json:"group_id"`
}

type PreviewInviteRequest struct {
	InviteToken string `json:"invite_token"`
}

// inviteListItem is the strict list wire shape: identity, creator, creation
// time, expiry, and revocation state only. The bearer token is never part of
// any response other than the single creation response.
type inviteListItem struct {
	ID            string     `json:"id"`
	CreatorUserID string     `json:"creator_user_id"`
	CreatedAt     time.Time  `json:"created_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	Revoked       *time.Time `json:"revoked"`
}

// inviteCreateResponse is returned exactly once when an invite is created. It
// carries the raw bearer token and the invite join path; every later listing
// and response only exposes the invite id and metadata. The invite_url is a
// relative path carrying the token in a fragment; clients prepend their own
// origin to build a shareable link.
type inviteCreateResponse struct {
	ID        string    `json:"id"`
	GroupID   string    `json:"group_id"`
	Token     string    `json:"token"`
	InviteURL string    `json:"invite_url"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CreateInvite issues a new bearer invite token for a group. The raw token is
// returned only here; persistence stores its SHA-256 hash. Any current group
// member may create invites (the equal-member model has no owner role).
func (a *GameAPI) CreateInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	var req CreateInviteRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	req.GroupID = strings.TrimSpace(req.GroupID)
	if err := ValidateID(req.GroupID, "group_id"); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_group_id", "Group ID is required")
		return
	}
	userID := GetUserIDFromContext(r)
	if !a.requireMember(w, r, req.GroupID, userID) {
		return
	}
	token, err := auth.GenerateOpaqueToken(32)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create invite")
		return
	}
	now := a.clock()
	invite := &models.GroupInvite{
		ID:            uuid.NewString(),
		GroupID:       req.GroupID,
		CreatorUserID: userID,
		TokenHash:     auth.HashToken(token),
		CreatedAt:     now,
		ExpiresAt:     now.Add(inviteTTL),
	}
	if err := a.groups.CreateInvite(r.Context(), invite, maxActiveInvitesPerGroup, maxInvitesPerUserPerDay); err != nil {
		switch {
		case errors.Is(err, groups.ErrNotMember):
			WriteError(w, http.StatusForbidden, "forbidden", "You are not a member of this group")
		case errors.Is(err, groups.ErrTooManyGroupInvites):
			WriteError(w, http.StatusConflict, "too_many_active_invites", "This group already has the maximum number of active invites")
		case errors.Is(err, groups.ErrTooManyUserInvites):
			WriteError(w, http.StatusTooManyRequests, "invite_creation_limit", "Invite creation limit reached for today")
		default:
			WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create invite")
		}
		return
	}
	// The invite URL is the relative join path with the bearer token in a
	// fragment (F-06): /group/join#invite=<token>. Clients prepend their
	// configured origin to produce the shareable absolute link.
	inviteURL := "/group/join#invite=" + token
	WriteJSON(w, http.StatusCreated, inviteCreateResponse{
		ID: invite.ID, GroupID: invite.GroupID, Token: token, InviteURL: inviteURL,
		CreatedAt: invite.CreatedAt, ExpiresAt: invite.ExpiresAt,
	})
}

// ListInvites returns the invites of a group for a current member. Token
// hashes are never selected, so the bearer value cannot leak through the list.
func (a *GameAPI) ListInvites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	groupID := strings.TrimSpace(r.URL.Query().Get("group_id"))
	if err := ValidateID(groupID, "group_id"); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_group_id", "Group ID is required")
		return
	}
	userID := GetUserIDFromContext(r)
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	invites, err := a.groups.ListInvitesByGroup(r.Context(), groupID, userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load invites")
		return
	}
	items := make([]inviteListItem, 0, len(invites))
	for _, invite := range invites {
		items = append(items, inviteListItem{
			ID: invite.ID, CreatorUserID: invite.CreatorUserID,
			CreatedAt: invite.CreatedAt, ExpiresAt: invite.ExpiresAt, Revoked: invite.RevokedAt,
		})
	}
	WriteJSON(w, http.StatusOK, items)
}

// RevokeInvite invalidates an invite immediately. Any current group member may
// revoke an invite of their group.
func (a *GameAPI) RevokeInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		MethodNotAllowed(w)
		return
	}
	inviteID := strings.TrimSpace(r.PathValue("inviteID"))
	if err := ValidateID(inviteID, "invite_id"); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_invite_id", "Invite ID is required")
		return
	}
	invite, err := a.groups.InviteByID(r.Context(), inviteID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to revoke invite")
		return
	}
	if invite == nil {
		WriteError(w, http.StatusNotFound, "invite_not_found", "Invite not found")
		return
	}
	userID := GetUserIDFromContext(r)
	if !a.requireMember(w, r, invite.GroupID, userID) {
		return
	}
	revoked, err := a.groups.RevokeInvite(r.Context(), inviteID, invite.GroupID, userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to revoke invite")
		return
	}
	if !revoked {
		WriteError(w, http.StatusNotFound, "invite_not_found", "Invite not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PreviewInvite returns non-sensitive group preview data for an invite token.
// The route is public and body-based so the bearer token never appears in a
// URL, path, or log line. Expired, revoked, and unknown tokens all resolve to
// the same generic 404 so the endpoint never reveals whether a token exists.
func (a *GameAPI) PreviewInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	var req PreviewInviteRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	token := req.InviteToken
	if !validInviteToken(token) {
		WriteError(w, http.StatusBadRequest, "invalid_invite_token", "invite_token must be a valid invite token")
		return
	}
	name, memberCount, found, err := a.groups.GroupPreviewByInviteTokenHash(r.Context(), auth.HashToken(token), a.clock())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to preview invite")
		return
	}
	if !found {
		WriteError(w, http.StatusNotFound, "invite_not_found", "Invite not found or expired")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"group_name": name, "member_count": memberCount})
}
