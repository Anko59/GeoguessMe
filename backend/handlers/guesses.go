package handlers

import (
	"encoding/json"
	"geoguessme/internal/auth"
	"geoguessme/internal/database"
	"net/http"
)

func GetMyGuesses(w http.ResponseWriter, r *http.Request) {
	// Auth check
	tokenString := r.Header.Get("Authorization")
	if tokenString == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	claims, err := auth.ValidateToken(tokenString)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	groupID := r.URL.Query().Get("group_id")
	if groupID == "" {
		http.Error(w, "Missing group_id", http.StatusBadRequest)
		return
	}

	query := `SELECT photo_id FROM guesses WHERE group_id = $1 AND user_id = $2`
	rows, err := database.DB.Query(r.Context(), query, groupID, claims.UserID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var photoIDs []string
	for rows.Next() {
		var pid string
		if err := rows.Scan(&pid); err != nil {
			continue
		}
		photoIDs = append(photoIDs, pid)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(photoIDs)
}
