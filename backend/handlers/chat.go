package handlers

import (
	"bytes"
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
	"geoguessme/internal/repository"
	chatrepo "geoguessme/internal/repository/chat"
	"geoguessme/internal/storage"

	"github.com/google/uuid"
)

// ChatAPI serves the chat slice from injected dependencies (PR 5). It owns
// transport only: request parsing, authorization delegation, service calls,
// and response writing. Persistence and WebSocket-ticket handling live in
// internal/repository/chat; the object store, hub, and clock are injected.
// ChatAPI replaced the package-level chat handlers and the HubInstance,
// MediaStore, and RuntimeConfig globals they read.
type ChatAPI struct {
	messages *chatrepo.Repository
	store    storage.ObjectStore
	cfg      *config.Config
	hub      *chatHub.Hub
	now      func() time.Time
}

// NewChatAPI constructs the chat transport with its explicit dependencies.
// now is the injectable clock (time.Now in production).
func NewChatAPI(messages *chatrepo.Repository, store storage.ObjectStore, cfg *config.Config, hub *chatHub.Hub, now func() time.Time) *ChatAPI {
	return &ChatAPI{messages: messages, store: store, cfg: cfg, hub: hub, now: now}
}

func (a *ChatAPI) GetGroupMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	userID := GetUserIDFromContext(r)
	groupID := r.URL.Query().Get("group_id")
	if groupID == "" {
		writeError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}

	if err := auth.VerifyGroupMembership(r.Context(), groupID, userID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden", "You are not a member of this group")
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
			writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load messages")
			return
		}
		page, err = a.messages.EnrichMessagesPageForViewer(r.Context(), page, userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load messages")
			return
		}
		if page.Items == nil {
			page.Items = []models.Message{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": page.Items, "next_cursor": page.NextCursor})
		return
	}

	// Stable cursor takes precedence; the legacy after_id message id is
	// resolved onto the same opaque cursor so reconnect callers that only know
	// the last message id keep working. A raw id must never reach the cursor
	// decoder, which expects an opaque base64 value.
	cursor := r.URL.Query().Get("cursor")
	if cursor == "" {
		resolved, err := a.messages.CursorAfterMessage(r.Context(), r.URL.Query().Get("after_id"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load messages")
			return
		}
		cursor = resolved
	}

	page, err := a.messages.GetGroupMessagesPageForViewer(r.Context(), groupID, cursor, limit, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load messages")
		return
	}
	if page.Items == nil {
		page.Items = []models.Message{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": page.Items, "next_cursor": page.NextCursor})
}

const maxChatMediaTextLength = 1000

// UploadChatMedia accepts one validated image or browser-recorded video and
// creates its message in the same database transaction as the attachment row.
// Challenge uploads must not be reused here: chat media has no location or
// expiry semantics and is readable only by active members of its group.
func (a *ChatAPI) UploadChatMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if a.store == nil || a.cfg == nil || a.hub == nil {
		writeError(w, http.StatusServiceUnavailable, "chat_unavailable", "Chat media is unavailable")
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
	if err := auth.VerifyGroupMembership(r.Context(), groupID, userID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden", "You are not a member of this group")
		return
	}
	content := strings.TrimSpace(r.FormValue("content"))
	if !utf8.ValidString(content) || utf8.RuneCountInString(content) > maxChatMediaTextLength {
		writeError(w, http.StatusBadRequest, "invalid_message", "Message is too long")
		return
	}
	var replyToID *string
	if value := strings.TrimSpace(r.FormValue("reply_to_id")); value != "" {
		if err := validateID(value, "reply_to_id"); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_message", "Reply target is invalid")
			return
		}
		replyToID = &value
	}
	file, header, err := r.FormFile("media")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_media", "A photo or video is required")
		return
	}
	defer file.Close()
	normalized, err := media.NormalizeChallengeUpload(file, header.Size, maxBytes, a.cfg.UploadMaxPixels)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_media", err.Error())
		return
	}

	now := a.now().UTC()
	asset := &models.ChatMedia{ID: uuid.NewString(), GroupID: groupID, UserID: userID, StorageKey: "chat-media/" + uuid.NewString(), MIMEType: normalized.MIMEType, ByteSize: int64(len(normalized.Data)), CreatedAt: now}
	if err := a.store.Put(r.Context(), asset.StorageKey, bytes.NewReader(normalized.Data), asset.ByteSize, asset.MIMEType); err != nil {
		writeError(w, http.StatusBadGateway, "storage_error", "Unable to store media")
		return
	}
	message := &models.Message{ID: uuid.NewString(), GroupID: groupID, UserID: userID, Kind: "media", ReplyToID: replyToID, Content: content, CreatedAt: now}
	if err := a.messages.CreateChatMediaMessage(r.Context(), message, asset); err != nil {
		if deleteErr := a.store.Delete(r.Context(), asset.StorageKey); deleteErr != nil {
			if queueErr := repository.EnqueueMediaDeletion(r.Context(), "manual", []string{asset.StorageKey}); queueErr != nil {
				slog.Error("failed to persist chat media upload compensation", "storage_key", asset.StorageKey, "delete_error", deleteErr, "enqueue_error", queueErr)
			}
		}
		if errors.Is(err, chatrepo.ErrInvalidMessageReply) {
			writeError(w, http.StatusBadRequest, "invalid_message", "Reply target is not in this group")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to create message")
		return
	}
	a.hub.BroadcastPersisted(*message)
	writeJSON(w, http.StatusCreated, message)
}

// ServeChatMedia streams an attachment only after checking that the requester
// is still a member of the message's group. The opaque storage key is never
// returned to clients and the response is deliberately non-cacheable.
func (a *ChatAPI) ServeChatMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	if a.store == nil {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable", "Chat media is unavailable")
		return
	}
	mediaID := r.PathValue("mediaID")
	if err := validateID(mediaID, "media_id"); err != nil {
		writeError(w, http.StatusBadRequest, "missing_media_id", "Media ID is required")
		return
	}
	asset, err := a.messages.GetChatMedia(r.Context(), mediaID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load media")
		return
	}
	if asset == nil {
		writeError(w, http.StatusNotFound, "not_found", "Media not found")
		return
	}
	if err := auth.VerifyGroupMembership(r.Context(), asset.GroupID, GetUserIDFromContext(r)); err != nil {
		writeError(w, http.StatusForbidden, "forbidden", "Media is not available")
		return
	}
	object, err := a.store.Get(r.Context(), asset.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			writeError(w, http.StatusGone, "media_removed", "The original media is no longer available")
			return
		}
		writeError(w, http.StatusBadGateway, "storage_error", "Unable to read media")
		return
	}
	defer object.Close()
	w.Header().Set("Content-Type", asset.MIMEType)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, object)
}

var allowedReactions = map[string]struct{}{
	// Custom reaction artwork keys offered by the UI.
	"like":       {},
	"love":       {},
	"laugh":      {},
	"wow":        {},
	"sad":        {},
	"spot-on":    {},
	"lost":       {},
	"mind-blown": {},
	"wrong-way":  {},
	"vacation":   {},
	// Legacy emoji reactions stay valid so existing data keeps working.
	"👍":  {},
	"❤️": {},
	"😂":  {},
	"😮":  {},
	"😢":  {},
	"🙏":  {},
}

type messageReactionRequest struct {
	Reaction string `json:"reaction"`
	Emoji    string `json:"emoji"`
}

// SetMessageReaction adds a reaction, while DELETE removes the same user's
// reaction for that reaction key. Both operations return the updated aggregate so
// every client can render the same reaction counts immediately.
func (a *ChatAPI) SetMessageReaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	messageID := strings.TrimSpace(r.PathValue("messageID"))
	if err := validateID(messageID, "message_id"); err != nil {
		writeError(w, http.StatusBadRequest, "missing_message_id", "Message ID is required")
		return
	}
	var req messageReactionRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Reaction = strings.TrimSpace(req.Reaction)
	if req.Reaction == "" {
		req.Reaction = strings.TrimSpace(req.Emoji)
	}
	if _, ok := allowedReactions[req.Reaction]; !ok {
		writeError(w, http.StatusBadRequest, "invalid_reaction", "Choose a supported reaction")
		return
	}

	userID := GetUserIDFromContext(r)
	message, err := a.messages.GetMessageForViewer(r.Context(), messageID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load message")
		return
	}
	if message == nil {
		writeError(w, http.StatusNotFound, "not_found", "Message not found")
		return
	}
	if message.Kind == "system" {
		writeError(w, http.StatusBadRequest, "invalid_reaction", "System messages cannot be reacted to")
		return
	}
	if err := auth.VerifyGroupMembership(r.Context(), message.GroupID, userID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden", "You are not a member of this group")
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
		writeError(w, http.StatusBadRequest, "invalid_reaction", "Choose a supported reaction")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to save reaction")
		return
	}
	updated, err := a.messages.GetMessageForViewer(r.Context(), messageID, userID)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load updated message")
		return
	}
	if a.hub != nil {
		broadcast := *updated
		broadcast.ReactionUpdate = &models.ReactionUpdate{
			UserID:   userID,
			Reaction: req.Reaction,
			Emoji:    req.Reaction,
			Active:   r.Method == http.MethodPut,
		}
		a.hub.BroadcastUpdate(broadcast)
	}
	writeJSON(w, http.StatusOK, updated)
}

func (a *ChatAPI) CreateWebSocketTicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	groupID := strings.TrimSpace(r.URL.Query().Get("group_id"))
	if groupID == "" {
		writeError(w, http.StatusBadRequest, "missing_group_id", "group_id is required")
		return
	}
	userID := GetUserIDFromContext(r)
	if err := auth.VerifyGroupMembership(r.Context(), groupID, userID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden", "You are not a member of this group")
		return
	}
	token, err := auth.GenerateOpaqueToken(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to create WebSocket ticket")
		return
	}
	now := a.now()
	if err := a.messages.CreateWebSocketTicket(r.Context(), uuid.NewString(), userID, groupID, auth.HashToken(token), now.Add(60*time.Second)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to create WebSocket ticket")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ticket": token, "expires_in": 60, "server_time": now})
}

// HandleChat upgrades an authenticated WebSocket connection after validating
// the one-time ticket scoped to the group. The hub is the injected instance
// owned by the composition root.
func (a *ChatAPI) HandleChat(w http.ResponseWriter, r *http.Request) {
	if a.hub == nil {
		writeError(w, http.StatusServiceUnavailable, "chat_unavailable", "Chat is unavailable")
		return
	}
	groupID := strings.TrimSpace(r.URL.Query().Get("group_id"))
	ticket := strings.TrimSpace(r.URL.Query().Get("ticket"))
	if groupID == "" || ticket == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "WebSocket ticket required")
		return
	}
	// Reject unknown origins before consuming the ticket so a bad origin can
	// never burn a valid one-time ticket.
	allowed := []string{}
	if a.cfg != nil {
		allowed = a.cfg.AllowedOrigins
	}
	if !chatHub.OriginAllowed(r.Header.Get("Origin"), allowed) {
		writeError(w, http.StatusForbidden, "origin_not_allowed", "Origin is not allowed")
		return
	}
	userID, err := a.messages.ConsumeWebSocketTicket(r.Context(), auth.HashToken(ticket), groupID)
	if err != nil || userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "WebSocket ticket is invalid or expired")
		return
	}
	chatHub.ServeWs(a.hub, w, r, groupID, userID, allowed)
}
