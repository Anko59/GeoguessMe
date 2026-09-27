package pins

import (
	"context"
	"errors"
	"time"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var ErrMapPinUnavailable = errors.New("map pin is unavailable or not unlocked")

// MapPinsForUser returns active pin content and its unlock routes, plus an
// inactive pin only when it is already equipped so the current selection can
// be understood and cleared.
func (r *Repository) MapPinsForUser(ctx context.Context, userID string) (models.MapPinCatalog, error) {
	catalog := models.MapPinCatalog{Pins: make([]models.MapPinChoice, 0)}
	rows, err := r.pool.Query(ctx, `
		SELECT p.pin_key, p.name, p.description, p.image_url,
		       c.challenge_key, c.name, c.description, u.unlocked_at,
		       (e.user_id IS NOT NULL)
		FROM map_pins p
	JOIN map_pin_challenges c ON c.pin_key = p.pin_key
		  AND (c.is_active OR EXISTS (
			  SELECT 1 FROM user_map_pin_unlocks owned
			  WHERE owned.user_id = $1 AND owned.challenge_key = c.challenge_key
		  ))
		LEFT JOIN user_map_pin_unlocks u
		  ON u.user_id = $1 AND u.pin_key = p.pin_key AND u.challenge_key = c.challenge_key
		LEFT JOIN user_equipped_map_pins e
		  ON e.user_id = $1 AND e.pin_key = p.pin_key
		WHERE p.is_active OR e.user_id IS NOT NULL
		ORDER BY p.display_order, p.pin_key, c.display_order, c.challenge_key`, userID)
	if err != nil {
		return models.MapPinCatalog{}, err
	}
	defer rows.Close()
	pinIndexes := make(map[string]int)
	for rows.Next() {
		var pin models.MapPinChoice
		var challenge models.MapPinChallengeProgress
		var unlockedAt pgtype.Timestamptz
		var selected bool
		if err := rows.Scan(
			&pin.Key, &pin.Name, &pin.Description, &pin.ImageURL,
			&challenge.Key, &challenge.Name, &challenge.Description, &unlockedAt, &selected,
		); err != nil {
			return models.MapPinCatalog{}, err
		}
		index, exists := pinIndexes[pin.Key]
		if !exists {
			pin.Challenges = make([]models.MapPinChallengeProgress, 0, 1)
			index = len(catalog.Pins)
			pinIndexes[pin.Key] = index
			catalog.Pins = append(catalog.Pins, pin)
		}
		if unlockedAt.Valid {
			challenge.UnlockedAt = &unlockedAt.Time
			catalog.Pins[index].Unlocked = true
		}
		catalog.Pins[index].Challenges = append(catalog.Pins[index].Challenges, challenge)
		if selected {
			key := pin.Key
			catalog.SelectedPinKey = &key
		}
	}
	if err := rows.Err(); err != nil {
		return models.MapPinCatalog{}, err
	}
	return catalog, nil
}

// EquippedMapPin returns the pin and exact challenge credited when the player
// equipped it. It returns nil when the player uses the standard marker.
func (r *Repository) EquippedMapPin(ctx context.Context, userID string) (*models.ProfileMapPin, error) {
	var pin models.ProfileMapPin
	err := r.pool.QueryRow(ctx, `
		SELECT p.pin_key, p.name, p.image_url, p.description,
		       c.challenge_key, c.name, c.description
		FROM user_equipped_map_pins e
		JOIN map_pins p ON p.pin_key = e.pin_key
		JOIN map_pin_challenges c ON c.challenge_key = e.challenge_key AND c.pin_key = e.pin_key
		WHERE e.user_id = $1`, userID).Scan(
		&pin.Key, &pin.Name, &pin.ImageURL, &pin.Description,
		&pin.UnlockedBy.Key, &pin.UnlockedBy.Name, &pin.UnlockedBy.Description,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &pin, nil
}

// EquipMapPin changes the selected pin only when the user owns an unlock for
// that active catalog entry. The foreign key keeps the selection tied to the
// specific unlock record used as its profile attribution.
func (r *Repository) EquipMapPin(ctx context.Context, userID, pinKey string) error {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO user_equipped_map_pins (user_id, pin_key, challenge_key)
		SELECT u.user_id, u.pin_key, u.challenge_key
		FROM user_map_pin_unlocks u
		JOIN map_pins p ON p.pin_key = u.pin_key AND p.is_active
		WHERE u.user_id = $1 AND u.pin_key = $2
		ORDER BY u.unlocked_at, u.challenge_key
		LIMIT 1
		ON CONFLICT (user_id) DO UPDATE
		SET pin_key = EXCLUDED.pin_key, challenge_key = EXCLUDED.challenge_key`, userID, pinKey)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrMapPinUnavailable
	}
	return nil
}

// ClearEquippedMapPin restores the standard marker without changing unlock history.
func (r *Repository) ClearEquippedMapPin(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM user_equipped_map_pins WHERE user_id = $1`, userID)
	return err
}

// AwardMapPinUnlock records one server-verified challenge completion. It is
// idempotent for a user/challenge pair; challenge-specific conditions are
// evaluated by the owning progression flow before calling this persistence seam.
func (r *Repository) AwardMapPinUnlock(ctx context.Context, userID, challengeKey string, unlockedAt time.Time) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO user_map_pin_unlocks (user_id, pin_key, challenge_key, unlocked_at)
		SELECT $1, c.pin_key, c.challenge_key, $3
		FROM map_pin_challenges c
		JOIN map_pins p ON p.pin_key = c.pin_key AND p.is_active
		WHERE c.challenge_key = $2 AND c.is_active
		ON CONFLICT (user_id, challenge_key) DO NOTHING`, userID, challengeKey, unlockedAt)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 1 {
		return true, nil
	}
	var alreadyUnlocked bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM user_map_pin_unlocks WHERE user_id = $1 AND challenge_key = $2
	)`, userID, challengeKey).Scan(&alreadyUnlocked); err != nil {
		return false, err
	}
	if alreadyUnlocked {
		return false, nil
	}
	return false, ErrMapPinUnavailable
}
