package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"geoguessme/internal/game"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"

	"github.com/google/uuid"
)

type GuessRequest struct {
	PhotoID string  `json:"photo_id"`
	Lat     float64 `json:"lat"`
	Long    float64 `json:"long"`
}

type GuessResponse struct {
	Score          int     `json:"score"`
	Distance       float64 `json:"distance"`
	ActualLat      float64 `json:"actual_lat"`
	ActualLong     float64 `json:"actual_long"`
	TotalUserScore int     `json:"total_user_score"` // Optional: return updated total score
}

func SubmitGuess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := GetUserIDFromContext(r)

	var req GuessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	photo, err := repository.GetPhoto(req.PhotoID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if photo == nil {
		http.Error(w, "Photo not found", http.StatusNotFound)
		return
	}

	// Prevent users from guessing their own photos
	if photo.UserID == userID {
		http.Error(w, "Cannot guess your own photo", http.StatusBadRequest)
		return
	}

	// Calculate distance and score
	distance := game.CalculateDistance(req.Lat, req.Long, photo.Lat, photo.Long)
	score := game.CalculateScore(distance)

	guess := &models.Guess{
		ID:        uuid.New().String(),
		PhotoID:   req.PhotoID,
		UserID:    userID,
		GroupID:   photo.GroupID,
		Lat:       req.Lat,
		Long:      req.Long,
		Score:     score,
		Distance:  distance,
		CreatedAt: time.Now(),
	}

	if err := repository.CreateGuess(guess); err != nil {
		http.Error(w, "Error saving guess", http.StatusInternalServerError)
		return
	}

	if err := repository.UpdateUserScore(userID, score); err != nil {
		// Log error but don't fail request?
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(GuessResponse{
		Score:      score,
		Distance:   distance,
		ActualLat:  photo.Lat,
		ActualLong: photo.Long,
	})
}
