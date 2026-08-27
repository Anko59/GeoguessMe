// Package party owns the Party Time persistence slice: starting a group
// party under the single-active-window and recharge rules, reporting the
// current window state, and resolving the double-points multiplier for a
// guess made during an active window. It mirrors the responsibility
// sub-package pattern of internal/repository/chat: one aggregate bound to an
// injected pool, plus transaction-scoped helpers shared with the gameplay
// slice.
package party

import (
	"context"
	"errors"
	"time"

	"geoguessme/internal/database"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Multiplier is the score multiplier applied to eligible guesses made while
// a party is active. Eligibility (the guesser posted a challenge during the
// active window) is resolved by ScoreMultiplier; this constant only states
// the factor.
const Multiplier = 2

// Sentinel persistence failures mapped to dedicated responses by the
// transport layer.
var (
	// ErrNotFound reports that the group does not exist.
	ErrNotFound = errors.New("group not found")
	// ErrPartyActive reports that a party is already running for the group.
	ErrPartyActive = errors.New("party time already active")
	// ErrPartyRecharging reports that the latest party ended but its recharge
	// cooldown has not elapsed yet.
	ErrPartyRecharging = errors.New("party time recharging")
)

// Window is one recorded party time of a group.
type Window struct {
	ID        string
	GroupID   string
	StartedBy string
	StartedAt time.Time
	EndsAt    time.Time
}

// Active reports whether the window covers the given instant.
func (w Window) Active(now time.Time) bool {
	return !w.StartedAt.After(now) && w.EndsAt.After(now)
}

// NextAvailableAt returns the earliest instant a new party may start after
// this window, given the configured recharge cooldown. With a zero cooldown
// a party may start again as soon as the previous one ends.
func (w Window) NextAvailableAt(cooldown time.Duration) time.Time {
	return w.EndsAt.Add(cooldown)
}

// State is the derived party view for one group at one instant.
type State struct {
	Window *Window // nil when the group never started a party
}

// Active reports whether a window covers the given instant.
func (s State) Active(now time.Time) bool {
	return s.Window != nil && s.Window.Active(now)
}

// Repository is the party persistence collection bound to one pool.
type Repository struct {
	pool database.Pool
}

// NewRepository returns a Repository bound to the given pool.
func NewRepository(pool database.Pool) *Repository {
	return &Repository{pool: pool}
}

// Querier is satisfied by both the connection pool and a transaction, so the
// same helpers serve pool-bound reads and transaction-scoped callers.
type Querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Start opens a new party window after enforcing the single-active-party and
// recharge rules. The group row is locked FOR UPDATE first so two concurrent
// starts serialize on the same row and each re-reads committed state before
// its rule check, which makes the 48h recharge rule race-free without
// additional locks or constraints.
func (r *Repository) Start(ctx context.Context, groupID, startedBy string, now time.Time, duration, cooldown time.Duration) (*Window, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var groupIDLocked string
	if err := tx.QueryRow(ctx, `SELECT id FROM groups WHERE id = $1 FOR UPDATE`, groupID).Scan(&groupIDLocked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	latest, err := LatestStarted(ctx, tx, groupID, now)
	if err != nil {
		return nil, err
	}
	if latest != nil {
		if latest.Active(now) {
			return nil, ErrPartyActive
		}
		if now.Before(latest.NextAvailableAt(cooldown)) {
			return nil, ErrPartyRecharging
		}
	}
	window := &Window{ID: uuid.NewString(), GroupID: groupID, StartedBy: startedBy, StartedAt: now, EndsAt: now.Add(duration)}
	if _, err := tx.Exec(ctx, `INSERT INTO group_party_times(id, group_id, started_by, started_at, ends_at) VALUES ($1, $2, $3, $4, $5)`, window.ID, window.GroupID, window.StartedBy, window.StartedAt, window.EndsAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return window, nil
}

// Status returns the latest party window of a group that has started at or
// before now (active or past), or nil when the group never started one.
func (r *Repository) Status(ctx context.Context, groupID string, now time.Time) (*Window, error) {
	return LatestStarted(ctx, r.pool, groupID, now)
}

// LatestStarted returns the latest party window of the group whose
// started_at is at or before until, or nil when none exists. It runs on any
// Querier so the pool-bound status read and transaction-scoped callers share
// one scan and one ordering definition.
func LatestStarted(ctx context.Context, q Querier, groupID string, until time.Time) (*Window, error) {
	var window Window
	err := q.QueryRow(ctx, `SELECT id, group_id, started_by, started_at, ends_at FROM group_party_times WHERE group_id = $1 AND started_at <= $2 ORDER BY started_at DESC LIMIT 1`, groupID, until).Scan(&window.ID, &window.GroupID, &window.StartedBy, &window.StartedAt, &window.EndsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &window, nil
}

// ScoreMultiplier resolves the score multiplier and doubling flag for one
// guess submitted by userID in groupID at instant now, inside the caller's
// transaction. A guess scores double exactly when a party window is active
// at the guess instant AND the guesser posted at least one challenge into
// the group since that window started — the posting incentive at the heart
// of the feature. No active window yields (1, false, nil).
func ScoreMultiplier(ctx context.Context, q Querier, groupID, userID string, now time.Time) (int, bool, error) {
	var posted bool
	err := q.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM photos pp
		WHERE pp.group_id = pt.group_id AND pp.user_id = $2 AND pp.created_at >= pt.started_at
	) FROM group_party_times pt
	WHERE pt.group_id = $1 AND pt.started_at <= $3 AND pt.ends_at > $3
	ORDER BY pt.started_at DESC LIMIT 1`, groupID, userID, now).Scan(&posted)
	if errors.Is(err, pgx.ErrNoRows) {
		return 1, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if posted {
		return Multiplier, true, nil
	}
	return 1, false, nil
}
