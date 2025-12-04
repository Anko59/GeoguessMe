package handlers

import (
	"encoding/json"
	"geoguessme/internal/repository"
	"net/http"
)

func GetGroupMembers(w http.ResponseWriter, r *http.Request) {
	// Auth is handled by middleware
	groupID := r.URL.Query().Get("id")
	if groupID == "" {
		http.Error(w, "Missing id", http.StatusBadRequest)
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
