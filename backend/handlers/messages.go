package handlers

import (
	"encoding/json"
	"geoguessme/internal/repository"
	"net/http"
)

func GetGroupMessages(w http.ResponseWriter, r *http.Request) {
	// Auth is handled by middleware
	groupID := r.URL.Query().Get("group_id")
	if groupID == "" {
		http.Error(w, "Missing group_id", http.StatusBadRequest)
		return
	}

	messages, err := repository.GetGroupMessages(groupID)
	if err != nil {
		http.Error(w, "Error fetching messages", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}
