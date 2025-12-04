package handlers

import (
	"encoding/json"
	"geoguessme/internal/repository"
	"net/http"
)

func GetPhotoDetails(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Auth is handled by middleware
	photoID := r.URL.Query().Get("id")
	if photoID == "" {
		http.Error(w, "Missing id", http.StatusBadRequest)
		return
	}

	photo, err := repository.GetPhoto(photoID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if photo == nil {
		http.Error(w, "Photo not found", http.StatusNotFound)
		return
	}

	guesses, err := repository.GetGuessesForPhoto(photoID)
	if err != nil {
		// Log error but return photo anyway? Or fail?
		// Let's return empty guesses if fail
		guesses = []repository.GuessWithUser{}
	}

	response := struct {
		ID        string                     `json:"id"`
		URL       string                     `json:"url"`
		Lat       float64                    `json:"lat"`
		Long      float64                    `json:"long"`
		Guesses   []repository.GuessWithUser `json:"guesses"`
		CreatedAt string                     `json:"created_at"`
	}{
		ID:        photo.ID,
		URL:       photo.URL,
		Lat:       photo.Lat,
		Long:      photo.Long,
		Guesses:   guesses,
		CreatedAt: photo.CreatedAt.String(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
