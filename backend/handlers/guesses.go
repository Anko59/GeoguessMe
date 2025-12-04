package handlers

import (
	"encoding/json"
	"geoguessme/internal/database"
	"net/http"
)

func GetMyGuesses(w http.ResponseWriter, r *http.Request) {
	userID := GetUserIDFromContext(r)

	groupID := r.URL.Query().Get("group_id")
	if groupID == "" {
		http.Error(w, "Missing group_id", http.StatusBadRequest)
		return
	}

	query := `SELECT photo_id FROM guesses WHERE group_id = $1 AND user_id = $2`
	rows, err := database.DB.Query(r.Context(), query, groupID, userID)
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
