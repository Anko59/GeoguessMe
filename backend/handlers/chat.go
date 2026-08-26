package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"geoguessme/internal/auth"
	chatHub "geoguessme/internal/chat"
	"geoguessme/internal/config"
	"geoguessme/internal/media"
	"geoguessme/internal/models"
	chatrepo "geoguessme/internal/repository/chat"
	"geoguessme/internal/repository/groups"
	"geoguessme/internal/storage"

	"github.com/google/uuid"
)

// ChatAPI serves the chat slice from injected dependencies (PR 5). It owns
// transport only: request parsing, authorization delegation, service calls,
// and response writing. Persistence and WebSocket-ticket handling live in
// internal/repository/chat; the object store, hub, and clock are injected.
// ChatAPI replaced the package-level chat handlers and the HubInstance,
// MediaStore, and RuntimeConfig globals they read. PR 6 additionally injects
// the durable media-deletion seam used by upload compensation, and PR 7
// injects the groups repository as the canonical membership gate.
type ChatAPI struct {
	messages *chatrepo.Repository
	groups   *groups.Repository
	store    storage.ObjectStore
	cfg      *config.Config
	hub      *chatHub.Hub
	now      func() time.Time
	media    UploadMediaStore
}

// NewChatAPI constructs the chat transport with its explicit dependencies.
// now is the injectable clock (time.Now in production) and media is the
// durable deletion-job and processing-job seam for upload lifecycle.
func NewChatAPI(messages *chatrepo.Repository, groups *groups.Repository, store storage.ObjectStore, cfg *config.Config, hub *chatHub.Hub, now func() time.Time, media UploadMediaStore) *ChatAPI {
	return &ChatAPI{messages: messages, groups: groups, store: store, cfg: cfg, hub: hub, now: now, media: media}
}

// requireMember is the chat slice's canonical membership gate. It delegates to
// the same groups repository the gameplay slice uses so no handler implements
// a subtly different membership rule. It writes the error response and
// returns false on failure.
func (a *ChatAPI) requireMember(w http.ResponseWriter, r *http.Request, groupID, userID string) bool {
	if err := a.groups.RequireMember(r.Context(), groupID, userID); err != nil {
		WriteError(w, http.StatusForbidden, "forbidden", "You are not a member of this group")
		return false
	}
	return true
}

func (a *ChatAPI) GetGroupMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}

	userID := GetUserIDFromContext(r)
	groupID := r.URL.Query().Get("group_id")
	if groupID == "" {
		WriteError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	if _, present := r.URL.Query()["after_id"]; present {
		WriteError(w, http.StatusBadRequest, "unsupported_parameter", "after_id is no longer supported; use cursor")
		return
	}

	if !a.requireMember(w, r, groupID, userID) {
		return
	}

	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	// Backward direction: before_id loads the page of messages older than the
	// referenced message (ascending), so the client can prepend history when
	// the user scrolls up. It takes precedence over the forward cursor and is
	// resolved scoped to the group.
	if beforeID := strings.TrimSpace(r.URL.Query().Get("before_id")); beforeID != "" {
		page, err := a.messages.GetGroupMessagesPageBefore(r.Context(), groupID, beforeID, limit)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load messages")
			return
		}
		page, err = a.messages.EnrichMessagesPageForViewer(r.Context(), page, userID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load messages")
			return
		}
		if page.Items == nil {
			page.Items = []models.Message{}
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": page.Items, "next_cursor": page.NextCursor, "stable_cursor": page.StableCursor})
		return
	}

	// The cursor parameter is the only forward-positioning mechanism: it is an
	// opaque base64 value (a raw message id must never reach the cursor
	// decoder). Reconnect catch-up sends the stable_cursor snapshot of the last
	// page; an empty cursor selects the most recent page.
	page, err := a.messages.GetGroupMessagesPageForViewer(r.Context(), groupID, r.URL.Query().Get("cursor"), limit, userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load messages")
		return
	}
	if page.Items == nil {
		page.Items = []models.Message{}
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": page.Items, "next_cursor": page.NextCursor, "stable_cursor": page.StableCursor})
}

const maxChatMediaTextLength = 1000

// UploadChatMedia accepts one validated image or browser-recorded video and
// creates its message in the same database transaction as the attachment row.
// Challenge uploads must not be reused here: chat media has no location or
// expiry semantics and is readable only by active members of its group.
func (a *ChatAPI) UploadChatMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	if a.store == nil || a.cfg == nil || a.hub == nil {
		WriteError(w, http.StatusServiceUnavailable, "chat_unavailable", "Chat media is unavailable")
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
	content := strings.TrimSpace(r.FormValue("content"))
	if !utf8.ValidString(content) || utf8.RuneCountInString(content) > maxChatMediaTextLength {
		WriteError(w, http.StatusBadRequest, "invalid_message", "Message is too long")
		return
	}
	var replyToID *string
	if value := strings.TrimSpace(r.FormValue("reply_to_id")); value != "" {
		if err := ValidateID(value, "reply_to_id"); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_message", "Reply target is invalid")
			return
		}
		replyToID = &value
	}
	file, header, err := r.FormFile("media")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "missing_media", "A photo or video is required")
		return
	}
	defer file.Close()
	normalized, err := media.NormalizeChallengeUpload(file, header.Size, maxBytes, a.cfg.UploadMaxPixels)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_media", err.Error())
		return
	}
	// Browser-recorded videos are processed asynchronously: the raw source is
	// quarantined and a processing job is queued instead of creating a visible
	// message immediately. Images keep the synchronous path below unchanged.
	if strings.HasPrefix(normalized.MIMEType, "video/") {
		a.queueVideoChatMessage(w, r, groupID, userID, normalized, content, replyToID)
		return
	}

	now := a.now().UTC()
	asset := &models.ChatMedia{ID: uuid.NewString(), GroupID: groupID, UserID: userID, StorageKey: "chat-media/" + uuid.NewString(), MIMEType: normalized.MIMEType, ByteSize: int64(len(normalized.Data)), CreatedAt: now}
	if err := a.store.Put(r.Context(), asset.StorageKey, bytes.NewReader(normalized.Data), asset.ByteSize, asset.MIMEType); err != nil {
		WriteError(w, http.StatusBadGateway, "storage_error", "Unable to store media")
		return
	}
	message := &models.Message{ID: uuid.NewString(), GroupID: groupID, UserID: userID, Kind: "media", ReplyToID: replyToID, Content: content, CreatedAt: now}
	if err := a.messages.CreateChatMediaMessage(r.Context(), message, asset); err != nil {
		if deleteErr := a.store.Delete(r.Context(), asset.StorageKey); deleteErr != nil {
			if queueErr := a.media.EnqueueMediaDeletion(r.Context(), "manual", []string{asset.StorageKey}); queueErr != nil {
				slog.Error("failed to persist chat media upload compensation", "storage_key", asset.StorageKey, "delete_error", deleteErr, "enqueue_error", queueErr)
			}
		}
		if errors.Is(err, chatrepo.ErrInvalidMessageReply) {
			WriteError(w, http.StatusBadRequest, "invalid_message", "Reply target is not in this group")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create message")
		return
	}
	a.hub.BroadcastPersisted(*message)
	WriteJSON(w, http.StatusCreated, message)
}

// queueVideoChatMessage stores a raw video under the private quarantine
// prefix, records a queued processing job carrying the chat message context,
// and answers 202 Accepted. No chat message or attachment row is created
// here: the worker creates the canonical object and the message atomically on
// success. A storage failure after the quarantine put compensates the object
// so no raw bytes are left unowned.
func (a *ChatAPI) queueVideoChatMessage(w http.ResponseWriter, r *http.Request, groupID, userID string, upload *media.Upload, content string, replyToID *string) {
	metadata, err := json.Marshal(models.ChatProcessingMetadata{GroupID: groupID, Content: content, ReplyToID: replyToID})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to queue media processing")
		return
	}
	jobID := uuid.NewString()
	quarantineKey := storage.QuarantineKey(jobID)
	if err := a.store.Put(r.Context(), quarantineKey, bytes.NewReader(upload.Data), int64(len(upload.Data)), upload.MIMEType); err != nil {
		WriteError(w, http.StatusBadGateway, "storage_error", "Unable to store media")
		return
	}
	job := &models.MediaProcessingJob{
		ID:            jobID,
		UserID:        userID,
		Kind:          models.MediaProcessingKindChat,
		Status:        models.MediaProcessingStatusQueued,
		QuarantineKey: quarantineKey,
		MIMEType:      upload.MIMEType,
		ByteSize:      int64(len(upload.Data)),
		GroupID:       groupID,
		QueuedAt:      a.now().UTC(),
		Metadata:      metadata,
	}
	if err := a.media.CreateProcessingJob(r.Context(), job); err != nil {
		a.compensateChatMediaDelete(r, quarantineKey)
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to queue media processing")
		return
	}
	WriteJSON(w, http.StatusAccepted, job.ToResponse())
}

// compensateChatMediaDelete removes a quarantine object after a failed upload
// so no orphaned bytes are left behind; a failed delete is enqueued as a
// durable deletion job instead of being dropped.
func (a *ChatAPI) compensateChatMediaDelete(r *http.Request, key string) {
	if err := a.store.Delete(r.Context(), key); err != nil {
		if enqueueErr := a.media.EnqueueMediaDeletion(r.Context(), "upload-compensation", []string{key}); enqueueErr != nil {
			slog.Error("failed to persist chat media upload compensation", "storage_key", key, "delete_error", err, "enqueue_error", enqueueErr)
		} else {
			slog.Warn("queued chat upload compensation after storage delete failure", "storage_key", key, "error", err)
		}
	}
}

// ServeChatMedia streams an attachment only after checking that the requester
// is still a member of the message's group. The opaque storage key is never
// returned to clients and the response is deliberately non-cacheable.
func (a *ChatAPI) ServeChatMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		MethodNotAllowed(w)
		return
	}
	if a.store == nil {
		WriteError(w, http.StatusServiceUnavailable, "storage_unavailable", "Chat media is unavailable")
		return
	}
	mediaID := r.PathValue("mediaID")
	if err := ValidateID(mediaID, "media_id"); err != nil {
		WriteError(w, http.StatusBadRequest, "missing_media_id", "Media ID is required")
		return
	}
	asset, err := a.messages.GetChatMedia(r.Context(), mediaID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load media")
		return
	}
	if asset == nil {
		WriteError(w, http.StatusNotFound, "not_found", "Media not found")
		return
	}
	if err := a.groups.RequireMember(r.Context(), asset.GroupID, GetUserIDFromContext(r)); err != nil {
		WriteError(w, http.StatusForbidden, "forbidden", "Media is not available")
		return
	}
	// Defense-in-depth: only canonical keys are ever served. Quarantine keys
	// hold raw asynchronous uploads and must never be streamable, even if a
	// database row referenced one.
	if !storage.IsCanonicalKey(asset.StorageKey) {
		WriteError(w, http.StatusGone, "media_removed", "The original media is no longer available")
		return
	}
	object, err := a.store.Get(r.Context(), asset.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			WriteError(w, http.StatusGone, "media_removed", "The original media is no longer available")
			return
		}
		WriteError(w, http.StatusBadGateway, "storage_error", "Unable to read media")
		return
	}
	defer object.Close()
	w.Header().Set("Content-Type", asset.MIMEType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, object)
}

// allowedReaction reports whether a reaction key is one of the supported
// reactions. It is a pure lookup (no package-level mutable state). The six
// legacy emoji values remain valid reaction keys so existing reaction rows
// keep rendering; the deprecated emoji request/response field alias was
// removed by the compatibility-removal PR, only the reaction key is accepted.
func allowedReaction(reaction string) bool {
	switch reaction {
	case "like", "love", "laugh", "wow", "sad", "spot-on", "lost", "mind-blown", "wrong-way", "vacation", "dislike", "cry", "kiss", "wink", "grin", "heart-eyes", "sunglasses", "angry", "confused", "sleepy", "clap", "pray", "fire", "party":
		return true
	// Legacy emoji reactions stay valid so existing data keeps working.
	case "👍", "❤️", "😂", "😮", "😢", "🙏":
		return true
	default:
		return false
	}
}

type messageReactionRequest struct {
	Reaction string `json:"reaction"`
}

// SetMessageReaction adds a reaction, while DELETE removes the same user's
// reaction for that reaction key. Both operations return the updated aggregate so
// every client can render the same reaction counts immediately.
func (a *ChatAPI) SetMessageReaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		MethodNotAllowed(w)
		return
	}
	messageID := strings.TrimSpace(r.PathValue("messageID"))
	if err := ValidateID(messageID, "message_id"); err != nil {
		WriteError(w, http.StatusBadRequest, "missing_message_id", "Message ID is required")
		return
	}
	var req messageReactionRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	req.Reaction = strings.TrimSpace(req.Reaction)
	if !allowedReaction(req.Reaction) {
		WriteError(w, http.StatusBadRequest, "invalid_reaction", "Choose a supported reaction")
		return
	}

	userID := GetUserIDFromContext(r)
	message, err := a.messages.GetMessageForViewer(r.Context(), messageID, userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load message")
		return
	}
	if message == nil {
		WriteError(w, http.StatusNotFound, "not_found", "Message not found")
		return
	}
	if message.Kind == "system" {
		WriteError(w, http.StatusBadRequest, "invalid_reaction", "System messages cannot be reacted to")
		return
	}
	if !a.requireMember(w, r, message.GroupID, userID) {
		return
	}

	if r.Method == http.MethodPut {
		err = a.messages.SetMessageReaction(r.Context(), messageID, userID, req.Reaction)
	} else {
		err = a.messages.DeleteMessageReaction(r.Context(), messageID, userID, req.Reaction)
	}
	// TOCTOU no-op: if the message is deleted between the load above and this
	// statement, the mutation is a harmless no-op (the insert carries a WHERE
	// EXISTS guard; the delete is naturally empty), so a stale write can never
	// affect an unknown message. The reload below then answers 500 — an
	// acceptable error path for a race that cannot occur while the message
	// still exists.
	if errors.Is(err, chatrepo.ErrInvalidReaction) {
		WriteError(w, http.StatusBadRequest, "invalid_reaction", "Choose a supported reaction")
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to save reaction")
		return
	}
	updated, err := a.messages.GetMessageForViewer(r.Context(), messageID, userID)
	if err != nil || updated == nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to load updated message")
		return
	}
	if a.hub != nil {
		broadcast := *updated
		broadcast.ReactionUpdate = &models.ReactionUpdate{
			UserID:   userID,
			Reaction: req.Reaction,
			Active:   r.Method == http.MethodPut,
		}
		a.hub.BroadcastUpdate(broadcast)
	}
	WriteJSON(w, http.StatusOK, updated)
}

func (a *ChatAPI) CreateWebSocketTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		MethodNotAllowed(w)
		return
	}
	groupID := strings.TrimSpace(r.URL.Query().Get("group_id"))
	if groupID == "" {
		WriteError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	userID := GetUserIDFromContext(r)
	if !a.requireMember(w, r, groupID, userID) {
		return
	}
	// Bind the ticket to the user's current auth_version so consumption rejects
	// any ticket minted before a password change, logout-all, or revocation.
	authVersion, err := a.messages.UserAuthVersion(r.Context(), userID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create WebSocket ticket")
		return
	}
	token, err := auth.GenerateOpaqueToken(32)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create WebSocket ticket")
		return
	}
	now := a.now()
	if err := a.messages.CreateWebSocketTicket(r.Context(), uuid.NewString(), userID, groupID, auth.HashToken(token), authVersion, now.Add(60*time.Second)); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal_error", "Unable to create WebSocket ticket")
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"ticket": token, "expires_in": 60, "server_time": now})
}

// HandleChat upgrades an authenticated WebSocket connection after validating
// the one-time ticket scoped to the group. The hub is the injected instance
// owned by the composition root.
func (a *ChatAPI) HandleChat(w http.ResponseWriter, r *http.Request) {
	if a.hub == nil {
		WriteError(w, http.StatusServiceUnavailable, "chat_unavailable", "Chat is unavailable")
		return
	}
	groupID := strings.TrimSpace(r.URL.Query().Get("group_id"))
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if groupID == "" || ticket == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "WebSocket ticket required")
		return
	}
	// Reject unknown origins before consuming the ticket so a bad origin can
	// never burn a valid one-time ticket.
	allowed := []string{}
	if a.cfg != nil {
		allowed = a.cfg.AllowedOrigins
	}
	if !chatHub.OriginAllowed(r.Header.Get("Origin"), allowed) {
		WriteError(w, http.StatusForbidden, "origin_not_allowed", "Origin is not allowed")
		return
	}
	userID, err := a.messages.ConsumeWebSocketTicket(r.Context(), auth.HashToken(ticket), groupID)
	if err != nil || userID == "" {
		WriteError(w, http.StatusUnauthorized, "unauthorized", "WebSocket ticket is invalid or expired")
		return
	}
	chatHub.ServeWs(a.hub, w, r, groupID, userID, allowed)
}
