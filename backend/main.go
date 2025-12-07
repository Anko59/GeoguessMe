package main

import (
	"fmt"
	"net/http"
	"time"

	"geoguessme/handlers"
	"geoguessme/internal/database"
	"geoguessme/internal/middleware"
)

func main() {
	database.Connect()
	database.InitSchema()
	handlers.InitChat()

	// Create a new ServeMux
	mux := http.NewServeMux()

	// Rate limiters
	authRateLimit := middleware.RateLimit(10, time.Minute)

	// Public endpoints with rate limiting
	mux.Handle("/signup", authRateLimit(http.HandlerFunc(handlers.Signup)))
	mux.Handle("/login", authRateLimit(http.HandlerFunc(handlers.Login)))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	// Protected endpoints (require auth)
	mux.HandleFunc("/user/groups", handlers.AuthMiddleware(handlers.GetUserGroups))
	mux.HandleFunc("/group/create", handlers.AuthMiddleware(handlers.CreateGroup))
	mux.HandleFunc("/group/join", handlers.AuthMiddleware(handlers.JoinGroup))
	mux.HandleFunc("/group/details", handlers.AuthMiddleware(handlers.GetGroupDetails))
	mux.HandleFunc("/group/members", handlers.AuthMiddleware(handlers.GetGroupMembers))
	mux.HandleFunc("/group/leaderboard", handlers.AuthMiddleware(handlers.GetLeaderboard))
	mux.HandleFunc("/group/messages", handlers.AuthMiddleware(handlers.GetGroupMessages))
	mux.HandleFunc("/photo/upload", handlers.AuthMiddleware(handlers.UploadPhoto))
	mux.HandleFunc("/photo/details", handlers.AuthMiddleware(handlers.GetPhotoDetails))
	mux.HandleFunc("/group/my_guesses", handlers.AuthMiddleware(handlers.GetMyGuesses))
	mux.HandleFunc("/guess", handlers.AuthMiddleware(handlers.SubmitGuess))
	mux.HandleFunc("/ws", handlers.HandleChat) // WebSocket has custom auth

	// Serve uploaded files
	fs := http.FileServer(http.Dir("./uploads"))
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", fs))

	// Apply global middleware
	handler := middleware.SecurityHeaders(mux)
	handler = middleware.CORS(handler)

	fmt.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		fmt.Printf("Server failed: %s\n", err)
	}
}
