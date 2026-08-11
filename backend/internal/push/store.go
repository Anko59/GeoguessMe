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
	Upsert(ctx context.Context, sub *Subscription, maxPerUser int) error
	Delete(ctx context.Context, userID, endpoint string) error
	ListForUser(ctx context.Context, userID string) ([]Subscription, error)
	ListForGroupUsers(ctx context.Context, groupID string, userIDs []string) ([]Subscription, error)
	DeleteByID(ctx context.Context, id string) error
	// CountSubscriptionsByUser reports how many active subscriptions one user
	// currently holds; the subscribe handler uses it for the per-user cap.
	CountSubscriptionsByUser(ctx context.Context, userID string) (int, error)
	// CountAllSubscriptions reports the total subscription count for the
	// push_subscriptions_total gauge.
	CountAllSubscriptions(ctx context.Context) (int64, error)
	// TouchSubscription refreshes last_used_at after a successful delivery so
	// the expiry cleanup never drops an actively used subscription.
	TouchSubscription(ctx context.Context, id string) error
	GroupTargets(ctx context.Context, groupID, excludeUserID string) ([]NotificationTarget, error)
	GroupName(ctx context.Context, groupID string) (string, error)
	Username(ctx context.Context, userID string) (string, error)
}

// pgStore implements Store against an injected connection pool.
type pgStore struct {
	pool database.Pool
}

// NewStore returns a Store backed by the given application database pool.
func NewStore(pool database.Pool) Store { return pgStore{pool: pool} }

// Upsert stores a subscription for a user, creating it or refreshing an
// existing (user_id, endpoint) row. maxPerUser bounds how many distinct
// endpoints one user may hold; refreshing an existing endpoint never counts
// against the cap. The cap check and the write run inside one transaction that
// locks the user row, so concurrent subscribes cannot slip past the limit.
func (s pgStore) Upsert(ctx context.Context, sub *Subscription, maxPerUser int) error {
	if sub.ID == "" {
		sub.ID = uuid.NewString()
	}
	if sub.CreatedAt.IsZero() {
		sub.CreatedAt = time.Now().UTC()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin upsert push subscription: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialize subscription writes per user so two concurrent requests cannot
	// both pass the cap check and insert a sixth endpoint.
	if _, err := tx.Exec(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, sub.UserID); err != nil {
		return fmt.Errorf("lock user for push subscription: %w", err)
	}
	var existing int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2`, sub.UserID, sub.Endpoint).Scan(&existing); err != nil {
		return fmt.Errorf("count existing push subscription: %w", err)
	}
	if existing == 0 && maxPerUser > 0 {
		var total int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM push_subscriptions WHERE user_id = $1`, sub.UserID).Scan(&total); err != nil {
			return fmt.Errorf("count push subscriptions: %w", err)
		}
		if total >= maxPerUser {
			return ErrSubscriptionLimit
		}
	}
	const query = `INSERT INTO push_subscriptions (id, user_id, endpoint, p256dh, auth, user_agent, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, endpoint) DO UPDATE SET
			p256dh = EXCLUDED.p256dh,
			auth = EXCLUDED.auth,
			user_agent = EXCLUDED.user_agent,
			created_at = EXCLUDED.created_at,
			last_used_at = NULL`
	if _, err := tx.Exec(ctx, query, sub.ID, sub.UserID, sub.Endpoint, sub.P256DH, sub.Auth, sub.UserAgent, sub.CreatedAt); err != nil {
		return fmt.Errorf("upsert push subscription: %w", err)
	}
	return tx.Commit(ctx)
}

// CountSubscriptionsByUser returns the number of stored subscriptions for one
// user. It powers the subscribe handler's friendly pre-check; the transactional
// cap inside Upsert remains the authoritative guard against races.
func (s pgStore) CountSubscriptionsByUser(ctx context.Context, userID string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM push_subscriptions WHERE user_id = $1`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count push subscriptions for user: %w", err)
	}
	return count, nil
}

// CountAllSubscriptions returns the total number of stored subscriptions.
func (s pgStore) CountAllSubscriptions(ctx context.Context) (int64, error) {
	var count int64
	err := s.pool.QueryRow(ctx, `SELECT count(*) FROM push_subscriptions`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count all push subscriptions: %w", err)
	}
	return count, nil
}

// TouchSubscription refreshes last_used_at after a successful delivery. A
// missing row is tolerated: the subscription may have been deleted by a
// concurrent cleanup between the send and this update.
func (s pgStore) TouchSubscription(ctx context.Context, id string) error {
	if _, err := s.pool.Exec(ctx, `UPDATE push_subscriptions SET last_used_at = CURRENT_TIMESTAMP WHERE id = $1`, id); err != nil {
		return fmt.Errorf("touch push subscription: %w", err)
	}
	return nil
}

func (s pgStore) Delete(ctx context.Context, userID, endpoint string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2`, userID, endpoint)
	if err != nil {
		return fmt.Errorf("delete push subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNoSubscription
	}
	return nil
}

func (s pgStore) ListForUser(ctx context.Context, userID string) ([]Subscription, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, user_id, endpoint, p256dh, auth, user_agent, created_at FROM push_subscriptions WHERE user_id = $1 ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptions(rows)
}

func (s pgStore) ListForGroupUsers(ctx context.Context, groupID string, userIDs []string) ([]Subscription, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT ps.id, ps.user_id, ps.endpoint, ps.p256dh, ps.auth, ps.user_agent, ps.created_at
		FROM push_subscriptions ps
		JOIN group_members gm ON gm.user_id = ps.user_id AND gm.group_id = $1
		WHERE ps.user_id = ANY($2)`, groupID, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptions(rows)
}

func (s pgStore) DeleteByID(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM push_subscriptions WHERE id = $1`, id)
	return err
}

func (s pgStore) GroupTargets(ctx context.Context, groupID, excludeUserID string) ([]NotificationTarget, error) {
	rows, err := s.pool.Query(ctx, `SELECT u.id, u.username
		FROM group_members gm
		JOIN users u ON u.id = gm.user_id AND u.deleted_at IS NULL
		LEFT JOIN group_notification_preferences np ON np.group_id = gm.group_id AND np.user_id = gm.user_id
		WHERE gm.group_id = $1 AND u.id <> $2 AND COALESCE(np.enabled, TRUE)`, groupID, excludeUserID)
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

func (s pgStore) GroupName(ctx context.Context, groupID string) (string, error) {
	var name string
	err := s.pool.QueryRow(ctx, `SELECT name FROM groups WHERE id = $1`, groupID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNoGroup
	}
	return name, err
}

func (s pgStore) Username(ctx context.Context, userID string) (string, error) {
	var username string
	err := s.pool.QueryRow(ctx, `SELECT username FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&username)
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

// ErrSubscriptionLimit indicates the user already holds the maximum number of
// distinct push subscriptions. The subscribe handler translates it into a
// 409 conflict response.
var ErrSubscriptionLimit = errors.New("push subscription limit reached")

// ErrNoGroup and ErrNoUser mark missing referenced entities during fan-out so
// the service can skip a notification instead of failing the triggering request.
var (
	ErrNoGroup = errors.New("group not found")
	ErrNoUser  = errors.New("user not found")
)
