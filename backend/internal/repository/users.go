package repository

import (
	"context"
	"geoguessme/internal/database"
	"geoguessme/internal/models"
)

func CreateUser(user *models.User) error {
	query := `INSERT INTO users (id, username, password, avatar, created_at) VALUES ($1, $2, $3, $4, $5)`
	_, err := database.DB.Exec(context.Background(), query, user.ID, user.Username, user.Password, user.Avatar, user.CreatedAt)
	return err
}

func GetUserByUsername(username string) (*models.User, error) {
	query := `SELECT id, username, password, avatar, created_at FROM users WHERE username = $1`
	var user models.User
	err := database.DB.QueryRow(context.Background(), query, username).Scan(&user.ID, &user.Username, &user.Password, &user.Avatar, &user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
