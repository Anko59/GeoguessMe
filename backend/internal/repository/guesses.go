package repository

import (
	"context"
	"errors"
	"geoguessme/internal/database"
	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
)

func CreateGuess(guess *models.Guess) error {
	query := `INSERT INTO guesses (id, photo_id, user_id, group_id, lat, long, score, distance, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := database.DB.Exec(context.Background(), query, guess.ID, guess.PhotoID, guess.UserID, guess.GroupID, guess.Lat, guess.Long, guess.Score, guess.Distance, guess.CreatedAt)
	return err
}

func GetPhoto(id string) (*models.Photo, error) {
	query := `SELECT id, user_id, group_id, url, lat, long, created_at, expires_at FROM photos WHERE id = $1`
	row := database.DB.QueryRow(context.Background(), query, id)

	var photo models.Photo
	err := row.Scan(&photo.ID, &photo.UserID, &photo.GroupID, &photo.URL, &photo.Lat, &photo.Long, &photo.CreatedAt, &photo.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &photo, nil
}

func UpdateUserScore(userID string, points int) error {
	query := `UPDATE users SET score = score + $1 WHERE id = $2`
	_, err := database.DB.Exec(context.Background(), query, points, userID)
	return err
}
