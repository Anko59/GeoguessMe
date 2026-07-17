package repository

import (
	"context"
	"errors"
	"time"

	"geoguessme/internal/database"
	"geoguessme/internal/game"
	"geoguessme/internal/models"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrForbidden         = errors.New("forbidden")
	ErrChallengeExpired  = errors.New("challenge expired")
	ErrViewNotFinished   = errors.New("viewing window is still open")
	ErrOwnPhoto          = errors.New("cannot use own challenge")
	ErrAlreadyGuessed    = errors.New("guess already submitted")
	ErrInvalidCoordinate = errors.New("invalid coordinate")
)

func CreatePhoto(photo *models.Photo) error {
	return CreatePhotoContext(context.Background(), photo)
}

func CreatePhotoContext(ctx context.Context, photo *models.Photo) error {
	_, err := database.DB.Exec(ctx, `INSERT INTO photos (id, user_id, group_id, url, storage_key, mime_type, byte_size, lat, long, lifecycle_status, created_at, expires_at, retention_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`, photo.ID, photo.UserID, photo.GroupID, photo.URL, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.Lat, photo.Long, photo.LifecycleStatus, photo.CreatedAt, photo.ExpiresAt, photo.RetentionAt)
	return err
}

func scanPhoto(row interface{ Scan(...any) error }) (*models.Photo, error) {
	var photo models.Photo
	err := row.Scan(&photo.ID, &photo.UserID, &photo.GroupID, &photo.URL, &photo.StorageKey, &photo.MIMEType, &photo.ByteSize, &photo.Lat, &photo.Long, &photo.LifecycleStatus, &photo.CreatedAt, &photo.ExpiresAt, &photo.RetentionAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &photo, nil
}

func GetPhoto(id string) (*models.Photo, error) {
	return GetPhotoContext(context.Background(), id)
}

func GetPhotoContext(ctx context.Context, id string) (*models.Photo, error) {
	return scanPhoto(database.DB.QueryRow(ctx, `SELECT id, user_id, group_id, url, storage_key, mime_type, byte_size, lat, long, lifecycle_status, created_at, expires_at, retention_at FROM photos WHERE id = $1`, id))
}

type ChallengeView struct {
	PhotoID       string    `json:"photo_id"`
	UserID        string    `json:"user_id"`
	AcceptedAt    time.Time `json:"accepted_at"`
	ViewExpiresAt time.Time `json:"view_expires_at"`
}

func AcceptChallenge(ctx context.Context, photoID, userID string, viewWindow time.Duration, now time.Time) (*models.Photo, *ChallengeView, error) {
	tx, err := database.DB.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	photo, err := scanPhoto(tx.QueryRow(ctx, `SELECT id, user_id, group_id, url, storage_key, mime_type, byte_size, lat, long, lifecycle_status, created_at, expires_at, retention_at FROM photos WHERE id = $1 FOR UPDATE`, photoID))
	if err != nil {
		return nil, nil, err
	}
	if photo == nil {
		return nil, nil, ErrNotFound
	}
	var member bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2)`, photo.GroupID, userID).Scan(&member); err != nil {
		return nil, nil, err
	}
	if !member {
		return nil, nil, ErrForbidden
	}
	if photo.UserID == userID {
		return nil, nil, ErrOwnPhoto
	}
	if photo.ExpiresAt.Before(now) || photo.LifecycleStatus == "removed" {
		return nil, nil, ErrChallengeExpired
	}
	var view ChallengeView
	err = tx.QueryRow(ctx, `SELECT photo_id, user_id, accepted_at, view_expires_at FROM challenge_views WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&view.PhotoID, &view.UserID, &view.AcceptedAt, &view.ViewExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		view = ChallengeView{PhotoID: photoID, UserID: userID, AcceptedAt: now, ViewExpiresAt: now.Add(viewWindow)}
		if view.ViewExpiresAt.After(photo.ExpiresAt) {
			view.ViewExpiresAt = photo.ExpiresAt
		}
		if _, err := tx.Exec(ctx, `INSERT INTO challenge_views(photo_id, user_id, accepted_at, view_expires_at) VALUES ($1, $2, $3, $4)`, view.PhotoID, view.UserID, view.AcceptedAt, view.ViewExpiresAt); err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	return photo, &view, nil
}

type GuessResult struct {
	Guess    models.Guess
	Photo    *models.Photo
	Existing bool
}

func CanViewResults(ctx context.Context, photoID, userID string, now time.Time) (*models.Photo, bool, error) {
	photo, err := GetPhotoContext(ctx, photoID)
	if err != nil {
		return nil, false, err
	}
	if photo == nil {
		return nil, false, ErrNotFound
	}
	var member bool
	if err := database.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2)`, photo.GroupID, userID).Scan(&member); err != nil {
		return nil, false, err
	}
	if !member {
		return nil, false, ErrForbidden
	}
	if photo.UserID == userID || !now.Before(photo.ExpiresAt) {
		return photo, true, nil
	}
	var guessed bool
	if err := database.DB.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM guesses WHERE photo_id = $1 AND user_id = $2)`, photoID, userID).Scan(&guessed); err != nil {
		return nil, false, err
	}
	return photo, guessed, nil
}

func SubmitGuess(ctx context.Context, photoID, userID string, lat, long float64, now time.Time) (*GuessResult, error) {
	if lat != lat || long != long || lat < -90 || lat > 90 || long < -180 || long > 180 {
		return nil, ErrInvalidCoordinate
	}
	const maxAttempts = 3
	var last error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		result, retry, err := submitGuessOnce(ctx, photoID, userID, lat, long, now)
		if !retry {
			return result, err
		}
		last = err
	}
	return nil, last
}

// submitGuessOnce performs a single guess attempt. It returns retry=true only
// for transient serialization/deadlock SQLSTATEs so the caller can try again
// within the documented limit.
func submitGuessOnce(ctx context.Context, photoID, userID string, lat, long float64, now time.Time) (*GuessResult, bool, error) {
	tx, err := database.DB.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	photo, err := scanPhoto(tx.QueryRow(ctx, `SELECT id, user_id, group_id, url, storage_key, mime_type, byte_size, lat, long, lifecycle_status, created_at, expires_at, retention_at FROM photos WHERE id = $1 FOR UPDATE`, photoID))
	if err != nil {
		return nil, isRetryable(err), err
	}
	if photo == nil {
		return nil, false, ErrNotFound
	}
	var member bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM group_members WHERE group_id = $1 AND user_id = $2)`, photo.GroupID, userID).Scan(&member); err != nil {
		return nil, isRetryable(err), err
	}
	if !member {
		return nil, false, ErrForbidden
	}
	if photo.UserID == userID {
		return nil, false, ErrOwnPhoto
	}
	var existing models.Guess
	err = tx.QueryRow(ctx, `SELECT id, photo_id, user_id, group_id, lat, long, score, distance, created_at FROM guesses WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&existing.ID, &existing.PhotoID, &existing.UserID, &existing.GroupID, &existing.Lat, &existing.Long, &existing.Score, &existing.Distance, &existing.CreatedAt)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, isRetryable(err), err
		}
		return &GuessResult{Guess: existing, Photo: photo, Existing: true}, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, isRetryable(err), err
	}
	if photo.ExpiresAt.Before(now) {
		return nil, false, ErrChallengeExpired
	}
	var viewExpiresAt time.Time
	if err := tx.QueryRow(ctx, `SELECT view_expires_at FROM challenge_views WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&viewExpiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, ErrForbidden
		}
		return nil, isRetryable(err), err
	}
	if now.Before(viewExpiresAt) {
		return nil, false, ErrViewNotFinished
	}
	distance := game.CalculateDistance(lat, long, photo.Lat, photo.Long)
	guess := models.Guess{ID: newID(), PhotoID: photoID, UserID: userID, GroupID: photo.GroupID, Lat: lat, Long: long, Score: game.CalculateScore(distance), Distance: distance, CreatedAt: now}
	if _, err := tx.Exec(ctx, `INSERT INTO guesses(id, photo_id, user_id, group_id, lat, long, score, distance, created_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, guess.ID, guess.PhotoID, guess.UserID, guess.GroupID, guess.Lat, guess.Long, guess.Score, guess.Distance, guess.CreatedAt); err != nil {
		// A concurrent duplicate lost the race; read the persisted winner.
		if isUniqueViolation(err) {
			return readExistingGuess(ctx, photoID, userID, photo)
		}
		return nil, isRetryable(err), err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, isRetryable(err), err
	}
	return &GuessResult{Guess: guess, Photo: photo}, false, nil
}

// readExistingGuess resolves the idempotent-duplicate case after a unique
// violation using a separate read so the original result is returned verbatim.
func readExistingGuess(ctx context.Context, photoID, userID string, photo *models.Photo) (*GuessResult, bool, error) {
	var existing models.Guess
	err := database.DB.QueryRow(ctx, `SELECT id, photo_id, user_id, group_id, lat, long, score, distance, created_at FROM guesses WHERE photo_id = $1 AND user_id = $2`, photoID, userID).Scan(&existing.ID, &existing.PhotoID, &existing.UserID, &existing.GroupID, &existing.Lat, &existing.Long, &existing.Score, &existing.Distance, &existing.CreatedAt)
	if err != nil {
		return nil, false, err
	}
	return &GuessResult{Guess: existing, Photo: photo, Existing: true}, false, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// isRetryable reports serialization/deadlock SQLSTATEs documented as safe to
// retry. All other errors are surfaced immediately.
func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	switch pgErr.Code {
	case "40001", "40P01": // serialization_failure, deadlock_detected
		return true
	}
	return false
}

func newID() string {
	return uuid.NewString()
}

type GuessWithUser struct {
	models.Guess
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
}

func GetGuessesForPhoto(photoID string) ([]GuessWithUser, error) {
	return GetGuessesForPhotoContext(context.Background(), photoID)
}

func GetGuessesForPhotoContext(ctx context.Context, photoID string) ([]GuessWithUser, error) {
	rows, err := database.DB.Query(ctx, `SELECT g.id, g.photo_id, g.user_id, g.group_id, g.lat, g.long, g.score, g.distance, g.created_at, u.username, u.avatar FROM guesses g JOIN users u ON g.user_id = u.id WHERE g.photo_id = $1 ORDER BY g.score DESC, g.created_at ASC`, photoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var guesses []GuessWithUser
	for rows.Next() {
		var g GuessWithUser
		if err := rows.Scan(&g.ID, &g.PhotoID, &g.UserID, &g.GroupID, &g.Lat, &g.Long, &g.Score, &g.Distance, &g.CreatedAt, &g.Username, &g.Avatar); err != nil {
			return nil, err
		}
		guesses = append(guesses, g)
	}
	return guesses, rows.Err()
}

func GetUserGuessedPhotoIDs(groupID, userID string) ([]string, error) {
	rows, err := database.DB.Query(context.Background(), `SELECT photo_id FROM guesses WHERE group_id = $1 AND user_id = $2 ORDER BY created_at`, groupID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
