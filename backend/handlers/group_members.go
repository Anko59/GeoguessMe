package handlers

import (
	"encoding/json"
	"geoguessme/internal/auth"
	"geoguessme/internal/repository"
	"net/http"
)

func GetGroupMembers(w http.ResponseWriter, r *http.Request) {
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

	members, err := repository.GetGroupMembers(groupID)
	if err != nil {
		http.Error(w, "Failed to fetch members", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(members)
}
