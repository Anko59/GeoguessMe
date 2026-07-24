package handlers

import (
	"crypto/rand"
	"fmt"
	"html"
	"math/big"
	"net/http"
	"strings"
	"time"

	"geoguessme/internal/auth"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	"geoguessme/internal/validation"

	"github.com/google/uuid"
)

type CreateGroupRequest struct {
	Name string `json:"name"`
}
type JoinGroupRequest struct {
	Code string `json:"code"`
}

func generateGroupCode() (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 6)
	for i := range b {
		value, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[value.Int64()]
	}
	return string(b), nil
}

func CreateGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req CreateGroupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := validation.ValidateGroupName(req.Name); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_group_name", err.Error())
		return
	}
	var code string
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		code, err = generateGroupCode()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Unable to create group")
			return
		}
		group, lookupErr := repository.GetGroupByCodeContext(r.Context(), code)
		if lookupErr != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Unable to create group")
			return
		}
		if group == nil {
			break
		}
	}
	if code == "" {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to create group")
		return
	}
	now := time.Now()
	group := &models.Group{ID: uuid.NewString(), Name: req.Name, Code: code, CreatedAt: now}
	if err := repository.CreateGroupAndMembership(r.Context(), group, GetUserIDFromContext(r)); err != nil {
		writeError(w, http.StatusConflict, "group_exists", "Unable to create group")
		return
	}
	writeJSON(w, http.StatusCreated, group)
}

func JoinGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req JoinGroupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Code = strings.ToUpper(strings.TrimSpace(req.Code))
	if err := validation.ValidateGroupCode(req.Code); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_group_code", err.Error())
		return
	}
	group, err := repository.GetGroupByCodeContext(r.Context(), req.Code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to join group")
		return
	}
	if group == nil {
		writeError(w, http.StatusNotFound, "group_not_found", "Group not found")
		return
	}
	if isMember, err := repository.IsGroupMemberContext(r.Context(), group.ID, GetUserIDFromContext(r)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to join group")
		return
	} else if isMember {
		// Invite links are intentionally idempotent: completing authentication
		// and replaying an invite must still open the existing group.
		writeJSON(w, http.StatusOK, group)
		return
	}
	if err := repository.AddGroupMemberContext(r.Context(), &models.GroupMember{GroupID: group.ID, UserID: GetUserIDFromContext(r), JoinedAt: time.Now()}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to join group")
		return
	}
	writeJSON(w, http.StatusOK, group)
}

func GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	groupID := r.URL.Query().Get("group_id")
	if groupID == "" {
		writeError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	if err := auth.VerifyGroupMembership(r.Context(), groupID, GetUserIDFromContext(r)); err != nil {
		writeError(w, http.StatusForbidden, "forbidden", "You are not a member of this group")
		return
	}
	entries, err := repository.GetGroupLeaderboardContext(r.Context(), groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load leaderboard")
		return
	}
	if entries == nil {
		entries = []repository.LeaderboardEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// invitePageTemplate is a minimal HTML shell with Open Graph meta tags so
// messengers render a rich preview when someone shares an invite link. It also
// includes a meta refresh that redirects the browser to the join page.
const invitePageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta property="og:title" content="%s">
<meta property="og:description" content="%s">
<meta property="og:image" content="%s/logo.png">
<meta property="og:type" content="website">
<meta property="og:site_name" content="GeoGuessMe">
<meta http-equiv="refresh" content="0;url=%s">
<title>%s</title>
</head>
<body></body>
</html>`

// HandleInvitePreview renders Open Graph link preview metadata for group
// invite links. Messengers and social platforms request the URL to produce a
// rich card; browsers are redirected to the actual join page via meta refresh.
// The route is unauthenticated so previews work even when the recipient is not
// logged in.
func HandleInvitePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	code := strings.ToUpper(strings.TrimSpace(r.PathValue("code")))
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing_code", "Group code is required")
		return
	}
	group, err := repository.GetGroupByCodeContext(r.Context(), code)
	if err != nil || group == nil {
		writeError(w, http.StatusNotFound, "group_not_found", "Group not found")
		return
	}
	inviterName := r.URL.Query().Get("from")
	if inviterName != "" {
		inviterName = html.EscapeString(inviterName)
	}
	groupName := html.EscapeString(group.Name)
	title := fmt.Sprintf("Join %s on GeoGuessMe", groupName)
	description := fmt.Sprintf("%s invites you to join the group %s on GeoGuessMe!", inviterName, groupName)
	if inviterName == "" {
		description = fmt.Sprintf("Join the group %s on GeoGuessMe!", groupName)
	}
	publicURL := ""
	if RuntimeConfig != nil {
		publicURL = strings.TrimRight(RuntimeConfig.PublicURL, "/")
	}
	redirectURL := fmt.Sprintf("%s/group/join?code=%s", publicURL, code)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(fmt.Sprintf(invitePageTemplate, html.EscapeString(title), html.EscapeString(description), publicURL, redirectURL, html.EscapeString(title))))
}
