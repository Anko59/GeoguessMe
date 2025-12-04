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

	http.HandleFunc("/signup", handlers.Signup)
	http.HandleFunc("/login", handlers.Login)
	http.HandleFunc("/user/groups", handlers.GetUserGroups)
	http.HandleFunc("/group/create", handlers.CreateGroup)
	http.HandleFunc("/group/join", handlers.JoinGroup)
	http.HandleFunc("/group/details", handlers.GetGroupDetails)
	http.HandleFunc("/group/leaderboard", handlers.GetLeaderboard)
	http.HandleFunc("/group/messages", handlers.GetGroupMessages)
	http.HandleFunc("/photo/upload", handlers.UploadPhoto)
	http.HandleFunc("/group/my_guesses", handlers.GetMyGuesses)
	http.HandleFunc("/guess", handlers.SubmitGuess)
	http.HandleFunc("/ws", handlers.HandleChat)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	// Serve uploaded files
	fs := http.FileServer(http.Dir("./uploads"))
	http.Handle("/uploads/", http.StripPrefix("/uploads/", fs))

	fmt.Println("Server starting on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Printf("Server failed: %s\n", err)
	}
}
