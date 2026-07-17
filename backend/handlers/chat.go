package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"geoguessme/internal/auth"
	"geoguessme/internal/chat"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"

	"github.com/google/uuid"
)

var HubInstance *chat.Hub

func InitChat() {
	HubInstance = chat.NewHub(func(_ context.Context, msg *models.Message) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return repository.SaveMessageContext(ctx, msg)
	})
	go HubInstance.Run()
}

func CreateWebSocketTicket(w http.ResponseWriter, r *http.Request) {
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
	if err := repository.CreateWebSocketTicket(r.Context(), uuid.NewString(), userID, groupID, auth.HashToken(token), time.Now().Add(60*time.Second)); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to create WebSocket ticket")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ticket": token, "expires_in": 60, "server_time": time.Now()})
}

func HandleChat(w http.ResponseWriter, r *http.Request) {
	if HubInstance == nil {
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
	if RuntimeConfig != nil {
		allowed = RuntimeConfig.AllowedOrigins
	}
	if !chat.OriginAllowed(r.Header.Get("Origin"), allowed) {
		writeError(w, http.StatusForbidden, "origin_not_allowed", "Origin is not allowed")
		return
	}
	userID, err := repository.ConsumeWebSocketTicket(r.Context(), auth.HashToken(ticket), groupID)
	if err != nil || userID == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized", "WebSocket ticket is invalid or expired")
		return
	}
	chat.ServeWs(HubInstance, w, r, groupID, userID, allowed)
}
