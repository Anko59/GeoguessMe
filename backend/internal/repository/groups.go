package repository

import (
	"context"

	"geoguessme/internal/database"
	"geoguessme/internal/models"
	"geoguessme/internal/repository/chat"
	"geoguessme/internal/repository/groups"
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
}

// NewRepository returns a Repository bound to the given pool, including the
// chat and gameplay persistence slices.
func NewRepository(pool database.Pool) *Repository {
	return &Repository{pool: pool, Chat: chat.NewRepository(pool), Groups: groups.NewRepository(pool)}
}

// UserGroups returns the groups a user belongs to, newest first. It is the
// read-only pilot slice (PR 4) migrated onto the injected repository seam; the
// implementation lives in the groups persistence slice.
func (r *Repository) UserGroups(ctx context.Context, userID string) ([]models.Group, error) {
	return r.Groups.UserGroups(ctx, userID)
}
