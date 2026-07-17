package handlers

import (
	"net/http"
	"strconv"

	"geoguessme/internal/auth"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
)

func GetGroupMessages(w http.ResponseWriter, r *http.Request) {
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

	// Stable cursor takes precedence; the legacy after_id message id is mapped
	// onto the same cursor so existing reconnect callers keep working.
	cursor := r.URL.Query().Get("cursor")
	if cursor == "" {
		cursor = r.URL.Query().Get("after_id")
	}
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	page, err := repository.GetGroupMessagesPage(r.Context(), groupID, cursor, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load messages")
		return
	}
	if page.Items == nil {
		page.Items = []models.Message{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": page.Items, "next_cursor": page.NextCursor})
}
