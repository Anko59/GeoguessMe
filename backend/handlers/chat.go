package handlers

import (
	"geoguessme/internal/auth"
	"geoguessme/internal/chat"
	"net/http"
)

var HubInstance *chat.Hub

func InitChat() {
	HubInstance = chat.NewHub()
	go HubInstance.Run()
}

func HandleChat(w http.ResponseWriter, r *http.Request) {
	// Auth via query param since WS doesn't support headers easily in standard API
	tokenString := r.URL.Query().Get("token")
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

	chat.ServeWs(HubInstance, w, r, groupID, claims.UserID)
}
