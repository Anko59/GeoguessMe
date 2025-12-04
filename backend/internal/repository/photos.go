package repository

import (
	"context"
	"geoguessme/internal/database"
	"geoguessme/internal/models"
)

func CreatePhoto(photo *models.Photo) error {
	query := `INSERT INTO photos (id, user_id, group_id, url, lat, long, created_at, expires_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := database.DB.Exec(context.Background(), query, photo.ID, photo.UserID, photo.GroupID, photo.URL, photo.Lat, photo.Long, photo.CreatedAt, photo.ExpiresAt)
	return err
}
