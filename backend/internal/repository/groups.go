package repository

import (
	"context"
	"errors"

	"geoguessme/internal/database"
	"geoguessme/internal/models"
	"geoguessme/internal/repository/chat"
	feedrepo "geoguessme/internal/repository/feed"
	"geoguessme/internal/repository/groups"
	"geoguessme/internal/repository/party"

	"github.com/jackc/pgx/v5"
)

// Repository is the concrete PostgreSQL persistence collection. PR 4
// introduces it as the dependency-injected seam: each migrated slice becomes a
// method on a repository bound to the injected pool, and the matching
// package-level function that read the database.DB global is removed. The chat
// slice lives in its own sub-package (Chat) since PR 5; PR 6 continues the
// split for the gameplay slice (Groups: membership/preferences, group and
// challenge data, and leaderboard/ranking queries). PR 7 converts the auth,
// profile, and cleanup slices onto methods here, leaving no package-level
// persistence function on the global pool.
//
// Instances are independent: two Repositories built on different pools never
// share state.
type Repository struct {
	pool database.Pool
	// Chat is the chat slice's persistence collection (messages, reactions,
	// chat media, and WebSocket tickets). The application composition root
	// hands it to the ChatAPI through App.Repos.Chat.
	Chat *chat.Repository
	// Groups is the gameplay slice's persistence collection (membership and
	// preferences, group and challenge data, leaderboard/ranking queries).
	// The application composition root hands it to the GameAPI through
	// App.Repos.Groups.
	Groups *groups.Repository
	// Party is the Party Time persistence collection (group party windows and
	// the double-points multiplier lookup). The application composition root
	// hands it to the party handler slice through App.Repos.Party.
	Party *party.Repository
}

// NewRepository returns a Repository bound to the given pool, including the
// chat, gameplay, and party persistence slices.
func NewRepository(pool database.Pool) *Repository {
	return &Repository{pool: pool, Chat: chat.NewRepository(pool), Groups: groups.NewRepository(pool), Party: party.NewRepository(pool)}
}

// UserGroups returns the groups a user belongs to, newest first. It is the
// read-only pilot slice (PR 4) migrated onto the injected repository seam; the
// implementation lives in the groups persistence slice.
func (r *Repository) UserGroups(ctx context.Context, userID string) ([]models.Group, error) {
	return r.Groups.UserGroups(ctx, userID)
}

// ExistingFeedChallenge checks the owner of an idempotent feed publication
// before media storage. A retry by the same owner is a no-op; another account
// cannot reuse the idempotency key.
func (r *Repository) ExistingFeedChallenge(ctx context.Context, challengeID, userID string) (bool, error) {
	var owner string
	err := r.pool.QueryRow(ctx, `SELECT user_id FROM public_challenges WHERE id = $1`, challengeID).Scan(&owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if owner != userID {
		return false, feedrepo.ErrConflict
	}
	return true, nil
}

// CreateFeedChallenge records one feed publication and all selected private
// group challenges in the same transaction. The caller stores the independent
// media objects first and compensates them if this transaction fails.
//
// The bool reports an idempotent retry that found the owner's existing feed
// row. It is deliberately resolved inside the transaction too, so two
// concurrent retries cannot create duplicate group challenges.
func (r *Repository) CreateFeedChallenge(ctx context.Context, challenge feedrepo.NewChallenge, photos []*models.Photo) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialize retries for the same client key before checking/inserting the
	// row. This avoids a unique-key race where the losing request would report
	// a 500 after the winner had already committed the same destinations.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, challenge.ID); err != nil {
		return false, err
	}

	var owner string
	err = tx.QueryRow(ctx, `SELECT user_id FROM public_challenges WHERE id = $1 FOR KEY SHARE`, challenge.ID).Scan(&owner)
	if err == nil {
		if owner != challenge.UserID {
			return false, feedrepo.ErrConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}

	if _, err := tx.Exec(ctx, `INSERT INTO public_challenges
		(id,user_id,caption,audience,storage_key,mime_type,preview,lat,long,created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, challenge.ID, challenge.UserID, challenge.Caption,
		challenge.Audience, challenge.StorageKey, challenge.MIMEType, challenge.Preview, challenge.Lat, challenge.Long, challenge.CreatedAt); err != nil {
		return false, err
	}

	if len(challenge.GroupIDs) > 0 {
		var authorized bool
		if err := tx.QueryRow(ctx, `SELECT NOT EXISTS (
			SELECT 1 FROM unnest($2::text[]) AS target(group_id)
			WHERE NOT EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = target.group_id AND gm.user_id = $1)
		)`, challenge.UserID, challenge.GroupIDs).Scan(&authorized); err != nil {
			return false, err
		}
		if !authorized {
			return false, feedrepo.ErrForbidden
		}
		if _, err := tx.Exec(ctx, `INSERT INTO public_challenge_groups(challenge_id,group_id) SELECT $1, unnest($2::text[])`, challenge.ID, challenge.GroupIDs); err != nil {
			return false, err
		}
	}
	if err := r.Groups.CreatePhotosTx(ctx, tx, photos); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return false, nil
}
