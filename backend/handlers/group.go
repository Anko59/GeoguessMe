package handlers

import (
	"crypto/rand"
	"encoding/json"
	"geoguessme/internal/auth"
	"math/big"
	"net/http"
	"time"

	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	"geoguessme/internal/validation"

	"github.com/google/uuid"
)

type CreateGroupRequest struct {
	Name string `json:"name"`
}

type JoinGroupRequest struct {
	Code string `json:"code"`
}

func generateGroupCode() (string, error) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 6)
	for i := range b {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[num.Int64()]
	}
	return string(b), nil
}

func CreateGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := GetUserIDFromContext(r)

	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Validate group name
	if err := validation.ValidateGroupName(req.Name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Generate unique code with retry logic
	var code string
	var err error
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		code, err = generateGroupCode()
		if err != nil {
			http.Error(w, "Error generating code", http.StatusInternalServerError)
			return
		}

		// Check if code already exists
		existingGroup, err := repository.GetGroupByCode(code)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if existingGroup == nil {
			break // Code is unique
		}

		if i == maxRetries-1 {
			http.Error(w, "Failed to generate unique code", http.StatusInternalServerError)
			return
		}
	}

	group := &models.Group{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Code:      code,
		CreatedAt: time.Now(),
	}

	if err := repository.CreateGroup(group); err != nil {
		http.Error(w, "Error creating group", http.StatusInternalServerError)
		return
	}

	// Add creator as member
	member := &models.GroupMember{
		GroupID:  group.ID,
		UserID:   userID,
		JoinedAt: time.Now(),
	}
	if err := repository.AddGroupMember(member); err != nil {
		http.Error(w, "Error adding member", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(group)
}

func JoinGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := GetUserIDFromContext(r)

	var req JoinGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Validate group code
	if err := validation.ValidateGroupCode(req.Code); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	group, err := repository.GetGroupByCode(req.Code)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if group == nil {
		http.Error(w, "Group not found", http.StatusNotFound)
		return
	}

	isMember, err := repository.IsGroupMember(group.ID, userID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if isMember {
		http.Error(w, "Already a member", http.StatusConflict)
		return
	}

	member := &models.GroupMember{
		GroupID:  group.ID,
		UserID:   userID,
		JoinedAt: time.Now(),
	}
	if err := repository.AddGroupMember(member); err != nil {
		http.Error(w, "Error joining group", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(group)
}

func GetLeaderboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := GetUserIDFromContext(r)

	groupID := r.URL.Query().Get("group_id")
	if groupID == "" {
		http.Error(w, "Missing group_id", http.StatusBadRequest)
		return
	}

	// Verify user is a member of the group
	if err := auth.VerifyGroupMembership(r.Context(), groupID, userID); err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	leaderboard, err := repository.GetGroupLeaderboard(groupID)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(leaderboard)
}
