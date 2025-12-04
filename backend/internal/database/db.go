package database

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var DB *pgxpool.Pool

func Connect() {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatalf("Unable to parse database URL: %v", err)
	}

	// Retry connection
	for i := 0; i < 5; i++ {
		DB, err = pgxpool.NewWithConfig(context.Background(), config)
		if err == nil {
			if err = DB.Ping(context.Background()); err == nil {
				fmt.Println("Connected to database")
				return
			}
		}
		fmt.Printf("Failed to connect to database (attempt %d/5): %v\n", i+1, err)
		time.Sleep(2 * time.Second)
	}

	log.Fatalf("Could not connect to database after retries: %v", err)
}

func InitSchema() {
	query := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password TEXT NOT NULL,
		avatar TEXT DEFAULT 'avatar.png',
		score INTEGER DEFAULT 0,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS groups (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		code TEXT UNIQUE NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS group_members (
		group_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		joined_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (group_id, user_id),
		FOREIGN KEY (group_id) REFERENCES groups(id),
		FOREIGN KEY (user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS photos (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		group_id TEXT NOT NULL,
		url TEXT NOT NULL,
		lat DOUBLE PRECISION NOT NULL,
		long DOUBLE PRECISION NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS guesses (
		id TEXT PRIMARY KEY,
		photo_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		lat DOUBLE PRECISION NOT NULL,
		long DOUBLE PRECISION NOT NULL,
		score INTEGER NOT NULL,
		distance DOUBLE PRECISION NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (photo_id) REFERENCES photos(id),
		FOREIGN KEY (user_id) REFERENCES users(id)
	);

	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		group_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		content TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (group_id) REFERENCES groups(id),
		FOREIGN KEY (user_id) REFERENCES users(id)
	);
	`

	_, err := DB.Exec(context.Background(), query)
	if err != nil {
		log.Fatalf("Failed to initialize schema: %v", err)
	}

	// Migration: Add group_id to guesses if it doesn't exist
	_, err = DB.Exec(context.Background(), `ALTER TABLE guesses ADD COLUMN IF NOT EXISTS group_id TEXT;`)
	if err != nil {
		log.Printf("Failed to migrate guesses table: %v", err)
	}

	fmt.Println("Schema initialized")
}
