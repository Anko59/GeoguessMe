package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"

	"geoguessme/internal/media"
	"geoguessme/internal/models"
	"geoguessme/internal/repository/groups"
	"geoguessme/internal/storage"
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

func (a *GameAPI) CreateGroup(w http.ResponseWriter, r *http.Request) {
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
		group, lookupErr := a.groups.ByCode(r.Context(), code)
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
	now := a.clock()
	group := &models.Group{ID: uuid.NewString(), Name: req.Name, Code: code, CreatedAt: now}
	if err := a.groups.Create(r.Context(), group, GetUserIDFromContext(r)); err != nil {
		writeError(w, http.StatusConflict, "group_exists", "Unable to create group")
		return
	}
	writeJSON(w, http.StatusCreated, group)
}

func (a *GameAPI) JoinGroup(w http.ResponseWriter, r *http.Request) {
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
	userID := GetUserIDFromContext(r)
	group, err := a.groups.ByCode(r.Context(), req.Code)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to join group")
		return
	}
	if group == nil {
		writeError(w, http.StatusNotFound, "group_not_found", "Group not found")
		return
	}
	if isMember, err := a.groups.IsMember(r.Context(), group.ID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to join group")
		return
	} else if isMember {
		// Invite links are intentionally idempotent: completing authentication
		// and replaying an invite must still open the existing group.
		writeJSON(w, http.StatusOK, group)
		return
	}
	if err := a.groups.AddMember(r.Context(), &models.GroupMember{GroupID: group.ID, UserID: userID, JoinedAt: a.clock()}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to join group")
		return
	}
	writeJSON(w, http.StatusOK, group)
}

func (a *GameAPI) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	groupID := r.URL.Query().Get("group_id")
	if groupID == "" {
		writeError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	userID := GetUserIDFromContext(r)
	period, validPeriod := groups.ParseLeaderboardPeriod(r.URL.Query().Get("period"))
	if r.URL.Query().Get("period") == "" {
		period = groups.LeaderboardAllTime
		validPeriod = true
	}
	if !validPeriod {
		writeError(w, http.StatusBadRequest, "invalid_period", "period must be week, month, or all")
		return
	}
	metric, validMetric := groups.ParseLeaderboardMetric(r.URL.Query().Get("metric"))
	if r.URL.Query().Get("metric") == "" {
		metric = groups.LeaderboardMetricTotal
		validMetric = true
	}
	if !validMetric {
		writeError(w, http.StatusBadRequest, "invalid_metric", "metric must be total, average, or elo")
		return
	}
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	entries, err := a.groups.LeaderboardForPeriod(r.Context(), groupID, period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load leaderboard")
		return
	}
	if entries == nil {
		entries = []groups.LeaderboardEntry{}
	}
	groups.SortLeaderboard(entries, metric)
	writeJSON(w, http.StatusOK, entries)
}

func (a *GameAPI) GetGroupDetails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	userID := GetUserIDFromContext(r)
	groupID := r.URL.Query().Get("id")
	if groupID == "" {
		writeError(w, http.StatusBadRequest, "missing_group_id", "id is required")
		return
	}
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	group, err := a.groups.ByID(r.Context(), groupID)
	if err != nil {
		writeError(w, http.StatusNotFound, "group_not_found", "Group not found")
		return
	}
	writeJSON(w, http.StatusOK, group)
}

func (a *GameAPI) GetGroupMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	userID := GetUserIDFromContext(r)
	groupID := r.URL.Query().Get("id")
	if groupID == "" {
		writeError(w, http.StatusBadRequest, "missing_group_id", "id is required")
		return
	}
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	members, err := a.groups.Members(r.Context(), groupID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load members")
		return
	}
	writeJSON(w, http.StatusOK, members)
}

// GroupReader is the narrow persistence contract the migrated group read
// handlers need. It is defined at the handler boundary so handler tests can
// substitute a fake instead of swapping the database.DB package global (PR 4
// pilot slice). The concrete *repository.Repository satisfies it.
type GroupReader interface {
	UserGroups(ctx context.Context, userID string) ([]models.Group, error)
}

// GroupAPI serves group read endpoints from injected dependencies. It is the
// target pattern for removing handler package globals: each migrated endpoint
// becomes a method here and the application composition root holds a
// *GroupAPI.
type GroupAPI struct {
	groups GroupReader
}

// NewGroupAPI constructs the migrated group read API with its reader.
func NewGroupAPI(groups GroupReader) *GroupAPI {
	return &GroupAPI{groups: groups}
}

// GetUserGroups lists the groups the authenticated user belongs to, newest
// first. It was migrated in PR 4 onto the injected GroupReader; the response
// shape, status codes, and error envelope are unchanged.
func (a *GroupAPI) GetUserGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	userID := GetUserIDFromContext(r)
	groups, err := a.groups.UserGroups(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load groups")
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

type groupNotificationRequest struct {
	Enabled bool `json:"enabled"`
}

func (a *GameAPI) GroupNotifications(w http.ResponseWriter, r *http.Request) {
	groupID := strings.TrimSpace(r.URL.Query().Get("group_id"))
	if err := validateID(groupID, "group_id"); err != nil {
		writeError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	userID := GetUserIDFromContext(r)
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		enabled, err := a.groups.NotificationPreference(r.Context(), groupID, userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load notification settings")
			return
		}
		writeJSON(w, http.StatusOK, groupNotificationRequest{Enabled: enabled})
	case http.MethodPut:
		var req groupNotificationRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if err := a.groups.SetNotificationPreference(r.Context(), groupID, userID, req.Enabled); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Unable to save notification settings")
			return
		}
		writeJSON(w, http.StatusOK, req)
	default:
		methodNotAllowed(w)
	}
}

func (a *GameAPI) GroupPhoto(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.serveGroupPhoto(w, r)
	case http.MethodPost:
		a.uploadGroupPhoto(w, r)
	default:
		methodNotAllowed(w)
	}
}

func (a *GameAPI) uploadGroupPhoto(w http.ResponseWriter, r *http.Request) {
	if a.store == nil || a.cfg == nil {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable", "Group photo storage is unavailable")
		return
	}
	maxBytes := a.cfg.UploadMaxBytes
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024*1024)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_upload", "Upload is too large or malformed")
		return
	}
	groupID := strings.TrimSpace(r.FormValue("group_id"))
	if err := validateID(groupID, "group_id"); err != nil {
		writeError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	userID := GetUserIDFromContext(r)
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_photo", "A photo is required")
		return
	}
	defer file.Close()
	normalized, err := media.NormalizeAvatar(file, header.Size, maxBytes, a.cfg.UploadMaxPixels)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_photo", err.Error())
		return
	}
	key := "groups/" + groupID + "/photo/" + uuid.NewString()
	if err := a.store.Put(r.Context(), key, bytes.NewReader(normalized.Data), int64(len(normalized.Data)), normalized.MIMEType); err != nil {
		writeError(w, http.StatusBadGateway, "storage_error", "Unable to store group photo")
		return
	}
	photo := &models.GroupPhoto{GroupID: groupID, StorageKey: key, MIMEType: normalized.MIMEType, ByteSize: int64(len(normalized.Data)), CreatedAt: a.clock()}
	previousKey, err := a.groups.SetGroupPhoto(r.Context(), photo)
	if err != nil {
		a.cleanupGroupPhoto(r, key)
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to save group photo")
		return
	}
	if previousKey != "" && previousKey != key {
		if err := a.store.Delete(r.Context(), previousKey); err != nil {
			if enqueueErr := a.media.EnqueueMediaDeletion(r.Context(), "group-photo-replacement", []string{previousKey}); enqueueErr != nil {
				slog.Error("failed to queue replaced group photo", "storage_key", previousKey, "delete_error", err, "enqueue_error", enqueueErr)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"group_id": photo.GroupID, "mime_type": photo.MIMEType, "byte_size": photo.ByteSize, "created_at": photo.CreatedAt})
}

func (a *GameAPI) cleanupGroupPhoto(r *http.Request, key string) {
	if err := a.store.Delete(r.Context(), key); err != nil {
		if enqueueErr := a.media.EnqueueMediaDeletion(r.Context(), "group-photo-compensation", []string{key}); enqueueErr != nil {
			slog.Error("failed to queue group photo compensation", "storage_key", key, "delete_error", err, "enqueue_error", enqueueErr)
		}
	}
}

func (a *GameAPI) serveGroupPhoto(w http.ResponseWriter, r *http.Request) {
	if a.store == nil {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable", "Group photo storage is unavailable")
		return
	}
	groupID := strings.TrimSpace(r.URL.Query().Get("group_id"))
	if err := validateID(groupID, "group_id"); err != nil {
		writeError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	userID := GetUserIDFromContext(r)
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	photo, err := a.groups.GroupPhoto(r.Context(), groupID)
	if err != nil || photo == nil {
		writeError(w, http.StatusNotFound, "not_found", "No group photo")
		return
	}
	object, err := a.store.Get(r.Context(), photo.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "No group photo")
			return
		}
		writeError(w, http.StatusBadGateway, "storage_error", "Unable to read group photo")
		return
	}
	defer object.Close()
	w.Header().Set("Content-Type", photo.MIMEType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, object)
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
func (a *GameAPI) HandleInvitePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	code := strings.ToUpper(strings.TrimSpace(r.PathValue("code")))
	if code == "" {
		writeError(w, http.StatusBadRequest, "missing_code", "Group code is required")
		return
	}
	group, err := a.groups.ByCode(r.Context(), code)
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
	if a.cfg != nil {
		publicURL = strings.TrimRight(a.cfg.PublicURL, "/")
	}
	redirectURL := fmt.Sprintf("%s/group/join?code=%s", publicURL, code)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(fmt.Sprintf(invitePageTemplate, html.EscapeString(title), html.EscapeString(description), publicURL, redirectURL, html.EscapeString(title))))
}
