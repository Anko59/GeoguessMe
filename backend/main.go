package main

import (
	"fmt"
	"net/http"

	"geoguessme/handlers"
	"geoguessme/internal/database"
)

func main() {
	database.Connect()
	database.InitSchema()
	handlers.InitChat()

	// Public endpoints (no auth required)
	http.HandleFunc("/signup", handlers.Signup)
	http.HandleFunc("/login", handlers.Login)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	// Protected endpoints (require auth)
	http.HandleFunc("/user/groups", handlers.AuthMiddleware(handlers.GetUserGroups))
	http.HandleFunc("/group/create", handlers.AuthMiddleware(handlers.CreateGroup))
	http.HandleFunc("/group/join", handlers.AuthMiddleware(handlers.JoinGroup))
	http.HandleFunc("/group/details", handlers.AuthMiddleware(handlers.GetGroupDetails))
	http.HandleFunc("/group/members", handlers.AuthMiddleware(handlers.GetGroupMembers))
	http.HandleFunc("/group/leaderboard", handlers.GetLeaderboard) // No auth currently
	http.HandleFunc("/group/messages", handlers.AuthMiddleware(handlers.GetGroupMessages))
	http.HandleFunc("/photo/upload", handlers.AuthMiddleware(handlers.UploadPhoto))
	http.HandleFunc("/group/my_guesses", handlers.AuthMiddleware(handlers.GetMyGuesses))
	http.HandleFunc("/guess", handlers.AuthMiddleware(handlers.SubmitGuess))
	http.HandleFunc("/ws", handlers.HandleChat) // WebSocket has custom auth

	// Serve uploaded files
	fs := http.FileServer(http.Dir("./uploads"))
	http.Handle("/uploads/", http.StripPrefix("/uploads/", fs))

	fmt.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Server failed: %s\n", err)
	}
}
