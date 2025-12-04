package handlers

import (
	"encoding/json"
	"geoguessme/internal/database"
	"net/http"
)

type UserGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

func GetUserGroups(w http.ResponseWriter, r *http.Request) {
	userID := GetUserIDFromContext(r)

	query := `SELECT g.id, g.name, g.code 
              FROM groups g 
              JOIN group_members gm ON g.id = gm.group_id 
              WHERE gm.user_id = $1
              ORDER BY gm.joined_at DESC`

	rows, err := database.DB.Query(r.Context(), query, userID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var groups []UserGroup
	for rows.Next() {
		var group UserGroup
		if err := rows.Scan(&group.ID, &group.Name, &group.Code); err != nil {
			continue
		}
		groups = append(groups, group)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(groups)
}
