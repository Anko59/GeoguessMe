package repository

import (
	"context"
	"time"

	"geoguessme/internal/database"
	"geoguessme/internal/elo"
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
// challenge data, and leaderboard/ranking queries).
//
// Slices not yet migrated (auth, profile, media cleanup) still use package
// functions over the database.DB global; the group helpers in this file that
// the profile slice still needs are thin adapters over the injected groups
// repository and are removed by the auth/profile migration (PR 7).
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

// SharesGroupContext reports whether two users are members of at least one
// common group. The profile slice still calls this package-level helper; the
// implementation lives in the groups persistence slice and is reached through
// the global pool until the auth/profile migration (PR 7).
func SharesGroupContext(ctx context.Context, userA, userB string) (bool, error) {
	return groups.NewRepository(database.DB).SharesGroup(ctx, userA, userB)
}

// IsGroupMemberContext reports whether a user is a member of a group. The
// auth slice (auth.VerifyGroupMembership) still calls this package-level
// helper; the implementation lives in the groups persistence slice and is
// reached through the global pool until the auth migration (PR 7).
func IsGroupMemberContext(ctx context.Context, groupID, userID string) (bool, error) {
	return groups.NewRepository(database.DB).IsMember(ctx, groupID, userID)
}

// EloStats and GlobalRankStats are re-exported for the not-yet-migrated
// profile slice, which still calls the package-level ranking helpers below.
type (
	EloStats        = groups.EloStats
	GlobalRankStats = groups.GlobalRankStats
)

// GetGlobalAverageRankContext ranks the player by average guess score among
// every player with at least one guess. It remains a profile-slice seam: the
// implementation lives in the groups persistence slice and is reached through
// the global pool until PR 7.
func GetGlobalAverageRankContext(ctx context.Context, userID string) (GlobalRankStats, error) {
	return groups.NewRepository(database.DB).GlobalAverageRank(ctx, userID)
}

// GetGlobalEloContext recomputes the global Elo ladder and returns the
// player's rating and competition rank. Profile-slice seam until PR 7.
func GetGlobalEloContext(ctx context.Context, userID string) (EloStats, error) {
	return groups.NewRepository(database.DB).GlobalElo(ctx, userID)
}

// LoadGlobalChallengesContext returns every challenge across all groups. It is
// exported for tests and any remaining external callers; the production
// implementation lives in the groups persistence slice.
func LoadGlobalChallengesContext(ctx context.Context) ([]elo.Challenge, error) {
	return groups.NewRepository(database.DB).GlobalChallenges(ctx)
}

// LoadChallengesContext returns every challenge for a group. Exported for
// tests; the production implementation lives in the groups persistence slice.
func LoadChallengesContext(ctx context.Context, groupID string, start *time.Time) ([]elo.Challenge, error) {
	return groups.NewRepository(database.DB).LoadChallenges(ctx, groupID, start)
}
