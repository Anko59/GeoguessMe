package handlers

import (
	"encoding/json"
	"geoguessme/internal/repository"
	"net/http"
)

func GetGroupDetails(w http.ResponseWriter, r *http.Request) {
	// Auth is handled by middleware
	groupID := r.URL.Query().Get("id")
	if groupID == "" {
		http.Error(w, "Missing id", http.StatusBadRequest)
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
