package handlers

import (
	"encoding/json"
	"geoguessme/internal/auth"
	"geoguessme/internal/repository"
	"net/http"
)

func GetGroupDetails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := GetUserIDFromContext(r)
	groupID := r.URL.Query().Get("id")
	if groupID == "" {
		http.Error(w, "Missing id", http.StatusBadRequest)
		return
	}

	// Verify user is a member of this group
	if err := auth.VerifyGroupMembership(r.Context(), groupID, userID); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	group, err := repository.GetGroupByID(groupID)
	if err != nil {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(group)
}
