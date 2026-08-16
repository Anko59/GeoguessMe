package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"

	"geoguessme/internal/auth"
	"geoguessme/internal/media"
	"geoguessme/internal/models"
	chatrepo "geoguessme/internal/repository/chat"
	"geoguessme/internal/repository/groups"
	"geoguessme/internal/storage"
	"geoguessme/internal/validation"

	"github.com/google/uuid"
)

type CreateGroupRequest struct {
	Name string `json:"name"`
}
type JoinGroupRequest struct {
	InviteToken string `json:"invite_token"`
	// Code is the legacy typed group code (F-06). It is accepted by the
	// decoder only so the handler can reject it with a dedicated 410 instead
	// of a generic body-validation error; typed-code joins are disabled.
	Code string `json:"code"`
}

// GetGroupReactionUsage returns the authenticated member's group-wide
// reaction counts for ordering the picker. It deliberately returns aggregate
// usage only, not message or member details.
func (a *ChatAPI) GetGroupReactionUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	groupID := strings.TrimSpace(r.URL.Query().Get("group_id"))
	if err := ValidateID(groupID, "group_id"); err != nil {
		WriteError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	if !a.requireMember(w, r, groupID, GetUserIDFromContext(r)) {
		return
	}
	usage, err := a.messages.ReactionUsageForGroup(r.Context(), groupID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load reaction usage")
		return
	}
	if usage == nil {
		usage = []chatrepo.ReactionUsage{}
	}
	WriteJSON(w, http.StatusOK, usage)
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
		MethodNotAllowed(w)
		return
	}
	var req CreateGroupRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := validation.ValidateGroupName(req.Name); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_group_name", err.Error())
		return
	}
	var code string
	var err error
	for attempt := 0; attempt < 8; attempt++ {
		code, err = generateGroupCode()
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create group")
			return
		}
		group, lookupErr := a.groups.ByCode(r.Context(), code)
		if lookupErr != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create group")
			return
		}
		if group == nil {
			break
		}
	}
	if code == "" {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create group")
		return
	}
	now := a.clock()
	group := &models.Group{ID: uuid.NewString(), Name: req.Name, Code: code, CreatedAt: now}
	if err := a.groups.Create(r.Context(), group, GetUserIDFromContext(r)); err != nil {
		WriteError(w, http.StatusConflict, "group_exists", "Unable to create group")
		return
	}
	WriteJSON(w, http.StatusCreated, group)
}

func (a *GameAPI) JoinGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	var req JoinGroupRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Code) != "" {
		// Legacy typed group codes were removed (F-06); the only accepted
		// join credential is a bearer invite token.
		WriteError(w, http.StatusGone, "legacy_group_code_disabled", "Group codes are no longer supported; use an invite link")
		return
	}
	token := req.InviteToken
	if token == "" {
		// No bearer invite token: the join credential is absent. Legacy
		// typed-group-code joins were removed (F-06).
		WriteError(w, http.StatusGone, "legacy_group_code_disabled", "Group codes are no longer supported; use an invite link")
		return
	}
	if !validInviteToken(token) {
		WriteError(w, http.StatusBadRequest, "invalid_invite_token", "invite_token must be a valid invite token")
		return
	}
	userID := GetUserIDFromContext(r)
	group, err := a.groups.JoinByInviteTokenHash(r.Context(), auth.HashToken(token), userID, a.clock())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to join group")
		return
	}
	if group == nil {
		WriteError(w, http.StatusNotFound, "invite_not_found", "Invite not found or expired")
		return
	}
	WriteJSON(w, http.StatusOK, group)
}

func (a *GameAPI) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	groupID := r.URL.Query().Get("group_id")
	if groupID == "" {
		WriteError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	userID := GetUserIDFromContext(r)
	period, validPeriod := groups.ParseLeaderboardPeriod(r.URL.Query().Get("period"))
	if r.URL.Query().Get("period") == "" {
		period = groups.LeaderboardAllTime
		validPeriod = true
	}
	if !validPeriod {
		WriteError(w, http.StatusBadRequest, "invalid_period", "period must be week, month, or all")
		return
	}
	metric, validMetric := groups.ParseLeaderboardMetric(r.URL.Query().Get("metric"))
	if r.URL.Query().Get("metric") == "" {
		metric = groups.LeaderboardMetricTotal
		validMetric = true
	}
	if !validMetric {
		WriteError(w, http.StatusBadRequest, "invalid_metric", "metric must be total, average, or elo")
		return
	}
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	entries, err := a.groups.LeaderboardForPeriod(r.Context(), groupID, period)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load leaderboard")
		return
	}
	if entries == nil {
		entries = []groups.LeaderboardEntry{}
	}
	groups.SortLeaderboard(entries, metric)
	WriteJSON(w, http.StatusOK, entries)
}

func (a *GameAPI) GetGroupDetails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	userID := GetUserIDFromContext(r)
	groupID := r.URL.Query().Get("id")
	if groupID == "" {
		WriteError(w, http.StatusBadRequest, "missing_group_id", "id is required")
		return
	}
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	group, err := a.groups.ByID(r.Context(), groupID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "group_not_found", "Group not found")
		return
	}
	WriteJSON(w, http.StatusOK, group)
}

func (a *GameAPI) GetGroupMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	userID := GetUserIDFromContext(r)
	groupID := r.URL.Query().Get("id")
	if groupID == "" {
		WriteError(w, http.StatusBadRequest, "missing_group_id", "id is required")
		return
	}
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	members, err := a.groups.Members(r.Context(), groupID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load members")
		return
	}
	WriteJSON(w, http.StatusOK, members)
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
		MethodNotAllowed(w)
		return
	}
	userID := GetUserIDFromContext(r)
	groups, err := a.groups.UserGroups(r.Context(), userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load groups")
		return
	}
	WriteJSON(w, http.StatusOK, groups)
}

type groupNotificationRequest struct {
	Enabled bool `json:"enabled"`
}

func (a *GameAPI) GroupNotifications(w http.ResponseWriter, r *http.Request) {
	groupID := strings.TrimSpace(r.URL.Query().Get("group_id"))
	if err := ValidateID(groupID, "group_id"); err != nil {
		WriteError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
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
			WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load notification settings")
			return
		}
		WriteJSON(w, http.StatusOK, groupNotificationRequest{Enabled: enabled})
	case http.MethodPut:
		var req groupNotificationRequest
		if !DecodeJSON(w, r, &req) {
			return
		}
		if err := a.groups.SetNotificationPreference(r.Context(), groupID, userID, req.Enabled); err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to save notification settings")
			return
		}
		WriteJSON(w, http.StatusOK, req)
	default:
		MethodNotAllowed(w)
	}
}

func (a *GameAPI) GroupPhoto(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.serveGroupPhoto(w, r)
	case http.MethodPost:
		a.uploadGroupPhoto(w, r)
	default:
		MethodNotAllowed(w)
	}
}

func (a *GameAPI) uploadGroupPhoto(w http.ResponseWriter, r *http.Request) {
	if a.store == nil || a.cfg == nil {
		WriteError(w, http.StatusServiceUnavailable, "storage_unavailable", "Group photo storage is unavailable")
		return
	}
	maxBytes := a.cfg.UploadMaxBytes
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+1024*1024)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_upload", "Upload is too large or malformed")
		return
	}
	groupID := strings.TrimSpace(r.FormValue("group_id"))
	if err := ValidateID(groupID, "group_id"); err != nil {
		WriteError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	userID := GetUserIDFromContext(r)
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	file, header, err := r.FormFile("photo")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "missing_photo", "A photo is required")
		return
	}
	defer file.Close()
	normalized, err := media.NormalizeAvatar(file, header.Size, maxBytes, a.cfg.UploadMaxPixels)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_photo", err.Error())
		return
	}
	key := "groups/" + groupID + "/photo/" + uuid.NewString()
	if err := a.store.Put(r.Context(), key, bytes.NewReader(normalized.Data), int64(len(normalized.Data)), normalized.MIMEType); err != nil {
		WriteError(w, http.StatusBadGateway, "storage_error", "Unable to store group photo")
		return
	}
	photo := &models.GroupPhoto{GroupID: groupID, StorageKey: key, MIMEType: normalized.MIMEType, ByteSize: int64(len(normalized.Data)), CreatedAt: a.clock()}
	previousKey, err := a.groups.SetGroupPhoto(r.Context(), photo)
	if err != nil {
		a.cleanupGroupPhoto(r, key)
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to save group photo")
		return
	}
	if previousKey != "" && previousKey != key {
		if err := a.store.Delete(r.Context(), previousKey); err != nil {
			if enqueueErr := a.media.EnqueueMediaDeletion(r.Context(), "group-photo-replacement", []string{previousKey}); enqueueErr != nil {
				slog.Error("failed to queue replaced group photo", "storage_key", previousKey, "delete_error", err, "enqueue_error", enqueueErr)
			}
		}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"group_id": photo.GroupID, "mime_type": photo.MIMEType, "byte_size": photo.ByteSize, "created_at": photo.CreatedAt})
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
		WriteError(w, http.StatusServiceUnavailable, "storage_unavailable", "Group photo storage is unavailable")
		return
	}
	groupID := strings.TrimSpace(r.URL.Query().Get("group_id"))
	if err := ValidateID(groupID, "group_id"); err != nil {
		WriteError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	userID := GetUserIDFromContext(r)
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	photo, err := a.groups.GroupPhoto(r.Context(), groupID)
	if err != nil || photo == nil {
		WriteError(w, http.StatusNotFound, "not_found", "No group photo")
		return
	}
	object, err := a.store.Get(r.Context(), photo.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			WriteError(w, http.StatusNotFound, "not_found", "No group photo")
			return
		}
		WriteError(w, http.StatusBadGateway, "storage_error", "Unable to read group photo")
		return
	}
	defer object.Close()
	w.Header().Set("Content-Type", photo.MIMEType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, object)
}
