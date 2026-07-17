package handlers

import (
	"geoguessme/internal/auth"
	"geoguessme/internal/repository"
	"net/http"
)

func GetGroupDetails(w http.ResponseWriter, r *http.Request) {
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

	// Verify user is a member of this group
	if err := auth.VerifyGroupMembership(r.Context(), groupID, userID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden", "You are not a member of this group")
		return
	}

	group, err := repository.GetGroupByID(groupID)
	if err != nil {
		writeError(w, http.StatusNotFound, "group_not_found", "Group not found")
		return
	}

	writeJSON(w, http.StatusOK, group)
}
