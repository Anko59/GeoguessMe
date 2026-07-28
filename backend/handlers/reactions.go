package handlers

import (
	"errors"
	"net/http"
	"strings"

	"geoguessme/internal/auth"
	"geoguessme/internal/repository"
)

var allowedReactionEmojis = map[string]struct{}{
	"👍":  {},
	"❤️": {},
	"😂":  {},
	"😮":  {},
	"😢":  {},
	"🙏":  {},
}

type messageReactionRequest struct {
	Emoji string `json:"emoji"`
}

// SetMessageReaction adds a reaction, while DELETE removes the same user's
// reaction for that emoji. Both operations return the updated aggregate so
// every client can render the same reaction counts immediately.
func SetMessageReaction(w http.ResponseWriter, r *http.Request) {
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
	req.Emoji = strings.TrimSpace(req.Emoji)
	if _, ok := allowedReactionEmojis[req.Emoji]; !ok {
		writeError(w, http.StatusBadRequest, "invalid_reaction", "Choose a supported emoji reaction")
		return
	}

	userID := GetUserIDFromContext(r)
	message, err := repository.GetMessageForViewer(r.Context(), messageID, userID)
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
		err = repository.SetMessageReaction(r.Context(), messageID, userID, req.Emoji)
	} else {
		err = repository.DeleteMessageReaction(r.Context(), messageID, userID, req.Emoji)
	}
	if errors.Is(err, repository.ErrMessageNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Message not found")
		return
	}
	if errors.Is(err, repository.ErrInvalidReaction) {
		writeError(w, http.StatusBadRequest, "invalid_reaction", "Choose a supported emoji reaction")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to save reaction")
		return
	}
	updated, err := repository.GetMessageForViewer(r.Context(), messageID, userID)
	if err != nil || updated == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load updated message")
		return
	}
	if HubInstance != nil {
		HubInstance.BroadcastUpdate(*updated)
	}
	writeJSON(w, http.StatusOK, updated)
}
