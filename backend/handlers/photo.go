package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"geoguessme/internal/models"
	"geoguessme/internal/repository"

	"github.com/google/uuid"
)

func UploadPhoto(w http.ResponseWriter, r *http.Request) {
	fmt.Println("UploadPhoto: Request received")
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID := GetUserIDFromContext(r)

	// Parse multipart form
	// 10MB max memory
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		fmt.Printf("UploadPhoto: Error parsing form: %v\n", err)
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	groupID := r.FormValue("group_id")
	latStr := r.FormValue("lat")
	longStr := r.FormValue("long")

	if groupID == "" || latStr == "" || longStr == "" {
		fmt.Println("UploadPhoto: Missing fields")
		http.Error(w, "Missing fields", http.StatusBadRequest)
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		fmt.Println("UploadPhoto: Invalid lat")
		http.Error(w, "Invalid latitude", http.StatusBadRequest)
		return
	}
	long, err := strconv.ParseFloat(longStr, 64)
	if err != nil {
		fmt.Println("UploadPhoto: Invalid long")
		http.Error(w, "Invalid longitude", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("photo")
	if err != nil {
		fmt.Printf("UploadPhoto: Error retrieving file: %v\n", err)
		http.Error(w, "Error retrieving file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save file
	filename := uuid.New().String() + filepath.Ext(header.Filename)
	filePath := filepath.Join("uploads", filename)
	fmt.Printf("UploadPhoto: Saving to %s\n", filePath)

	dst, err := os.Create(filePath)
	if err != nil {
		fmt.Printf("UploadPhoto: Error creating file: %v\n", err)
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		fmt.Printf("UploadPhoto: Error copying file: %v\n", err)
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	photo := &models.Photo{
		ID:        uuid.New().String(),
		UserID:    userID,
		GroupID:   groupID,
		URL:       "/uploads/" + filename,
		Lat:       lat,
		Long:      long,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour), // Default expiry
	}

	fmt.Println("UploadPhoto: Creating DB record")
	if err := repository.CreatePhoto(photo); err != nil {
		fmt.Printf("UploadPhoto: DB Error: %v\n", err)
		http.Error(w, "Error creating photo record", http.StatusInternalServerError)
		return
	}

	// Broadcast new photo message to group
	msg := models.Message{
		GroupID: groupID,
		UserID:  userID, // Use the actual uploader's ID
		Content: fmt.Sprintf("NEW_PHOTO:%s:%s", photo.ID, photo.URL),
	}
	fmt.Println("UploadPhoto: Broadcasting message")
	if HubInstance != nil {
		HubInstance.Broadcast(msg)
		fmt.Println("UploadPhoto: Broadcast sent")
	} else {
		fmt.Println("UploadPhoto: Hub is nil!")
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(photo)
	fmt.Println("UploadPhoto: Success")
}
