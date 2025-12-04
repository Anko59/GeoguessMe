package handlers

import (
	"encoding/json"
	"geoguessme/internal/auth"
	"geoguessme/internal/repository"
	"net/http"
)

func GetGroupDetails(w http.ResponseWriter, r *http.Request) {
	// Auth check
	tokenString := r.Header.Get("Authorization")
	if tokenString == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	_, err := auth.ValidateToken(tokenString)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

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
