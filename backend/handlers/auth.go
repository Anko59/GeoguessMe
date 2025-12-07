package handlers

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"geoguessme/internal/auth"
	"geoguessme/internal/models"
	"geoguessme/internal/repository"
	"geoguessme/internal/validation"
	"math/big"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type SignupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	} `json:"user"`
}

func buildAuthResponse(user *models.User, token string) AuthResponse {
	return AuthResponse{
		Token: token,
		User: struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		}{
			ID:       user.ID,
			Username: user.Username,
		},
	}
}

func Signup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Validate username
	if err := validation.ValidateUsername(req.Username); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Validate password
	if err := validation.ValidatePassword(req.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	existingUser, err := repository.GetUserByUsername(req.Username)
	if err != nil {
		fmt.Printf("Signup error (GetUserByUsername): %v\n", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	if existingUser != nil {
		http.Error(w, "Username already taken", http.StatusConflict)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		fmt.Printf("Signup error (bcrypt): %v\n", err)
		http.Error(w, "Error processing request", http.StatusInternalServerError)
		return
	}

	// Random avatar selection using crypto/rand (avatar.png, avatar2.png ... avatar10.png)
	avatarNum, err := rand.Int(rand.Reader, big.NewInt(10))
	if err != nil {
		fmt.Printf("Signup error (rand): %v\n", err)
		http.Error(w, "Error processing request", http.StatusInternalServerError)
		return
	}

	avatarFile := "avatar.png"
	if avatarNum.Int64() > 0 {
		avatarFile = fmt.Sprintf("avatar%d.png", avatarNum.Int64()+1) // avatar2.png to avatar10.png
	}

	user := models.User{
		ID:        uuid.New().String(),
		Username:  req.Username,
		Password:  string(hashedPassword),
		Avatar:    avatarFile,
		CreatedAt: time.Now(),
	}

	if err := repository.CreateUser(&user); err != nil {
		fmt.Printf("Signup error (CreateUser): %v\n", err)
		http.Error(w, "Error creating user", http.StatusInternalServerError)
		return
	}

	token, err := auth.GenerateToken(user.ID)
	if err != nil {
		fmt.Printf("Signup error (GenerateToken): %v\n", err)
		http.Error(w, "Error generating token", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buildAuthResponse(&user, token))
}

func Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	user, err := repository.GetUserByUsername(req.Username)
	if err != nil {
		http.Error(w, "Authentication failed", http.StatusUnauthorized)
		return
	}
	if user == nil {
		http.Error(w, "Authentication failed", http.StatusUnauthorized)
		return
	}

	if !auth.CheckPasswordHash(req.Password, user.Password) {
		http.Error(w, "Authentication failed", http.StatusUnauthorized)
		return
	}

	token, err := auth.GenerateToken(user.ID)
	if err != nil {
		http.Error(w, "Authentication failed", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(buildAuthResponse(user, token))
}
