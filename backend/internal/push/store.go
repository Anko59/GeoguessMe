package push

import (
	"context"
	"errors"
	"fmt"
	"time"

	"geoguessme/internal/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Subscription is a stored Web Push subscription for one user. A user may have
// several (one per device/browser). P256DH and Auth are the base64url
// credentials the browser returned from PushManager.subscribe.
type Subscription struct {
	ID        string
	UserID    string
	Endpoint  string
	P256DH    string
	Auth      string
	UserAgent string
	CreatedAt time.Time
}

// NotificationTarget is a user who should receive a push for a group event.
type NotificationTarget struct {
	UserID   string
	Username string
}

// Store persists push subscriptions and resolves notification targets. It is an
// interface so the fan-out service is unit-testable without a database.
type Store interface {
	Upsert(ctx context.Context, sub *Subscription) error
	Delete(ctx context.Context, userID, endpoint string) error
	ListForUser(ctx context.Context, userID string) ([]Subscription, error)
	ListForUsers(ctx context.Context, userIDs []string) ([]Subscription, error)
	DeleteByID(ctx context.Context, id string) error
	GroupTargets(ctx context.Context, groupID, excludeUserID string) ([]NotificationTarget, error)
	GroupName(ctx context.Context, groupID string) (string, error)
	Username(ctx context.Context, userID string) (string, error)
}

// pgStore implements Store against the shared connection pool.
type pgStore struct{}

// NewStore returns a Store backed by the application database pool.
func NewStore() Store { return pgStore{} }

func (pgStore) Upsert(ctx context.Context, sub *Subscription) error {
	if sub.ID == "" {
		sub.ID = uuid.NewString()
	}
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = time.Now().UTC()
	}
	const query = `INSERT INTO push_subscriptions (id, user_id, endpoint, p256dh, auth, user_agent, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, endpoint) DO UPDATE SET p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth, user_agent = EXCLUDED.user_agent`
	_, err := database.DB.Exec(ctx, query, sub.ID, sub.UserID, sub.Endpoint, sub.P256DH, sub.Auth, sub.UserAgent, sub.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert push subscription: %w", err)
	}
	return nil
}

func (pgStore) Delete(ctx context.Context, userID, endpoint string) error {
	tag, err := database.DB.Exec(ctx, `DELETE FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2`, userID, endpoint)
	if err != nil {
		return fmt.Errorf("delete push subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoSubscription
	}
	return nil
}

func (pgStore) ListForUser(ctx context.Context, userID string) ([]Subscription, error) {
	rows, err := database.DB.Query(ctx, `SELECT id, user_id, endpoint, p256dh, auth, user_agent, created_at FROM push_subscriptions WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptions(rows)
}

func (pgStore) ListForUsers(ctx context.Context, userIDs []string) ([]Subscription, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	rows, err := database.DB.Query(ctx, `SELECT id, user_id, endpoint, p256dh, auth, user_agent, created_at FROM push_subscriptions WHERE user_id = ANY($1)`, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptions(rows)
}

func (pgStore) DeleteByID(ctx context.Context, id string) error {
	_, err := database.DB.Exec(ctx, `DELETE FROM push_subscriptions WHERE id = $1`, id)
	return err
}

func (pgStore) GroupTargets(ctx context.Context, groupID, excludeUserID string) ([]NotificationTarget, error) {
	rows, err := database.DB.Query(ctx, `SELECT u.id, u.username FROM group_members gm JOIN users u ON u.id = gm.user_id AND u.deleted_at IS NULL WHERE gm.group_id = $1 AND u.id <> $2`, groupID, excludeUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []NotificationTarget
	for rows.Next() {
		var t NotificationTarget
		if err := rows.Scan(&t.UserID, &t.Username); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

func (pgStore) GroupName(ctx context.Context, groupID string) (string, error) {
	var name string
	err := database.DB.QueryRow(ctx, `SELECT name FROM groups WHERE id = $1`, groupID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoGroup
	}
	return name, err
}

func (pgStore) Username(ctx context.Context, userID string) (string, error) {
	var username string
	err := database.DB.QueryRow(ctx, `SELECT username FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&username)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoUser
	}
	return username, err
}

func scanSubscriptions(rows pgx.Rows) ([]Subscription, error) {
	var subs []Subscription
	for rows.Next() {
		var s Subscription
		if err := rows.Scan(&s.ID, &s.UserID, &s.Endpoint, &s.P256DH, &s.Auth, &s.UserAgent, &s.CreatedAt); err != nil {
			return nil, err
		}
		subs = append(subs, s)
	}
	return subs, rows.Err()
}

// ErrNoSubscription indicates no subscription matched the delete criteria.
var ErrNoSubscription = errors.New("no push subscription matched")

// ErrNoGroup and ErrNoUser mark missing referenced entities during fan-out so
// the service can skip a notification instead of failing the triggering request.
var (
	ErrNoGroup = errors.New("group not found")
	ErrNoUser  = errors.New("user not found")
)
