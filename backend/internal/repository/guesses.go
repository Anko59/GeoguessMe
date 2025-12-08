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

type GuessWithUser struct {
	models.Guess
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

func GetGuessesForPhoto(photoID string) ([]GuessWithUser, error) {
	query := `
		SELECT g.id, g.photo_id, g.user_id, g.group_id, g.lat, g.long, g.score, g.distance, g.created_at,
		       u.username, u.avatar
		FROM guesses g
		JOIN users u ON g.user_id = u.id
		WHERE g.photo_id = $1
		ORDER BY g.score DESC
	`
	rows, err := database.DB.Query(context.Background(), query, photoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var guesses []GuessWithUser
	for rows.Next() {
		var g GuessWithUser
		if err := rows.Scan(
			&g.ID, &g.PhotoID, &g.UserID, &g.GroupID, &g.Lat, &g.Long, &g.Score, &g.Distance, &g.CreatedAt,
			&g.Username, &g.Avatar,
		); err != nil {
			continue
		}
		guesses = append(guesses, g)
	}
	return guesses, nil
}

func GetUserGuessedPhotoIDs(groupID, userID string) ([]string, error) {
	query := `SELECT photo_id FROM guesses WHERE group_id = $1 AND user_id = $2`
	rows, err := database.DB.Query(context.Background(), query, groupID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var photoIDs []string
	for rows.Next() {
		var pid string
		if err := rows.Scan(&pid); err != nil {
			continue
		}
		photoIDs = append(photoIDs, pid)
	}
	return photoIDs, nil
}
