package repository

import (
	"context"
	"errors"
	"sort"

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

// ReserveFeedChallenge serializes one idempotent publication before any
// object-store writes. A new reservation keeps its transaction open until the
// handler either commits the database rows after storage succeeds or rolls the
// transaction back after a storage failure. Retries wait on the same advisory
// lock and therefore never overwrite or clean up another request's objects.
func (r *Repository) ReserveFeedChallenge(ctx context.Context, challenge feedrepo.NewChallenge, photos []*models.Photo) (reservation feedrepo.PublicationReservation, err error) {
	if challenge.Audience == "" {
		challenge.Audience = "public"
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	reservationState := &feedPublicationReservation{tx: tx, groups: r.Groups}
	defer func() {
		if err != nil {
			_ = reservationState.Rollback(ctx)
		}
	}()
	// Serialize retries for the same client key before checking/inserting the
	// row and before either request writes canonical storage keys.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, challenge.ID); err != nil {
		return nil, err
	}

	var owner, caption, audience string
	var lat, long float64
	err = tx.QueryRow(ctx, `SELECT user_id,caption,audience,lat,long FROM public_challenges WHERE id = $1 FOR KEY SHARE`, challenge.ID).Scan(&owner, &caption, &audience, &lat, &long)
	if err == nil {
		if owner != challenge.UserID {
			return nil, feedrepo.ErrConflict
		}
		storedGroups, queryErr := feedChallengeGroups(ctx, tx, challenge.ID)
		if queryErr != nil {
			return nil, queryErr
		}
		requestedGroups := sortedGroupIDs(challenge.GroupIDs)
		if caption != challenge.Caption || audience != challenge.Audience || lat != challenge.Lat || long != challenge.Long || !sameGroupIDs(storedGroups, requestedGroups) {
			return nil, feedrepo.ErrConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		reservationState.tx = nil
		reservationState.existing = true
		reservationState.groupIDs = storedGroups
		return reservationState, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	challenge.GroupIDs = sortedGroupIDs(challenge.GroupIDs)
	return reservationState, nil
}

type feedPublicationReservation struct {
	tx       pgx.Tx
	groups   *groups.Repository
	existing bool
	groupIDs []string
}

func (r *feedPublicationReservation) Existing() bool { return r.existing }

func (r *feedPublicationReservation) GroupIDs() []string { return append([]string(nil), r.groupIDs...) }

func (r *feedPublicationReservation) Rollback(ctx context.Context) error {
	if r.tx == nil {
		return nil
	}
	tx := r.tx
	r.tx = nil
	err := tx.Rollback(ctx)
	if errors.Is(err, pgx.ErrTxClosed) {
		return nil
	}
	return err
}

func (r *feedPublicationReservation) Create(ctx context.Context, challenge feedrepo.NewChallenge, photos []*models.Photo) error {
	if r.existing || r.tx == nil {
		return errors.New("feed publication reservation is already finalized")
	}
	challenge.GroupIDs = sortedGroupIDs(challenge.GroupIDs)
	if _, err := r.tx.Exec(ctx, `INSERT INTO public_challenges
		(id,user_id,caption,audience,storage_key,mime_type,preview,lat,long,created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, challenge.ID, challenge.UserID, challenge.Caption,
		challenge.Audience, challenge.StorageKey, challenge.MIMEType, challenge.Preview, challenge.Lat, challenge.Long, challenge.CreatedAt); err != nil {
		return err
	}

	if len(challenge.GroupIDs) > 0 {
		var authorized bool
		if err := r.tx.QueryRow(ctx, `SELECT NOT EXISTS (
			SELECT 1 FROM unnest($2::text[]) AS target(group_id)
			WHERE NOT EXISTS (SELECT 1 FROM group_members gm WHERE gm.group_id = target.group_id AND gm.user_id = $1)
		)`, challenge.UserID, challenge.GroupIDs).Scan(&authorized); err != nil {
			return err
		}
		if !authorized {
			return feedrepo.ErrForbidden
		}
		if _, err := r.tx.Exec(ctx, `INSERT INTO public_challenge_groups(challenge_id,group_id) SELECT $1, unnest($2::text[])`, challenge.ID, challenge.GroupIDs); err != nil {
			return err
		}
	}
	if err := r.groups.CreatePhotosTx(ctx, r.tx, photos); err != nil {
		return err
	}
	if err := r.tx.Commit(ctx); err != nil {
		r.tx = nil
		return err
	}
	r.tx = nil
	r.existing = false
	r.groupIDs = sortedGroupIDs(challenge.GroupIDs)
	return nil
}

func feedChallengeGroups(ctx context.Context, tx pgx.Tx, challengeID string) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT group_id FROM public_challenge_groups WHERE challenge_id=$1 ORDER BY group_id`, challengeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := []string{}
	for rows.Next() {
		var groupID string
		if err := rows.Scan(&groupID); err != nil {
			return nil, err
		}
		groups = append(groups, groupID)
	}
	return groups, rows.Err()
}

func sortedGroupIDs(groupIDs []string) []string {
	ids := append([]string(nil), groupIDs...)
	sort.Strings(ids)
	return ids
}

func sameGroupIDs(left, right []string) bool {
	return len(left) == len(right) && func() bool {
		for i := range left {
			if left[i] != right[i] {
				return false
			}
		}
		return true
	}()
}
