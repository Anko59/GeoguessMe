package integration_test

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"geoguessme/internal/database"
	"geoguessme/internal/repository"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// testDB connects to the isolated integration database (TEST_DATABASE_URL) and
// routes repository functions through that pool for the duration of the test. It
// is a separate connection pool from the running backend process but points at
// the same database, so row locks and transactions interoperate correctly.
func testDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; requires the isolated integration DB")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	require.NoError(t, pool.Ping(context.Background()))
	prev := database.DB
	database.DB = pool
	t.Cleanup(func() {
		database.DB = prev
		pool.Close()
	})
	return pool
}

// seedRetentionPhoto inserts a minimal user, group, and active photo with a
// known storage key and future retention, returning the photo id and key. Rows
// use per-run unique handles and are removed on test completion so the suite is
// re-runnable against a persistent stack. Future retention keeps the running
// backend's hourly sweep from touching this photo during the test.
func seedRetentionPhoto(t *testing.T, db *pgxpool.Pool) (photoID, storageKey string) {
	t.Helper()
	ctx := context.Background()
	handle := unique("retention")
	userID := "retention-user-" + handle
	groupID := "retention-group-" + handle
	photoID = "retention-photo-" + handle
	storageKey = "retention-media/" + handle
	_, err := db.Exec(ctx,
		`INSERT INTO users (id, username, password, email, email_normalized) VALUES ($1, $2, 'x', $3, $3)`,
		userID, handle, handle+"@example.test")
	require.NoError(t, err)
	_, err = db.Exec(ctx,
		`INSERT INTO groups (id, name, code) VALUES ($1, $2, $3)`,
		groupID, "Retention "+handle, "code-"+handle)
	require.NoError(t, err)
	_, err = db.Exec(ctx,
		`INSERT INTO photos (id, user_id, group_id, storage_key, lat, long, lifecycle_status, expires_at, retention_at) VALUES ($1, $2, $3, $4, 0, 0, 'ready', CURRENT_TIMESTAMP + interval '1 hour', CURRENT_TIMESTAMP + interval '30 days')`,
		photoID, userID, groupID, storageKey)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM photos WHERE id = $1`, photoID)
		_, _ = db.Exec(context.Background(), `DELETE FROM groups WHERE id = $1`, groupID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return photoID, storageKey
}

// deleteDeletionJobs removes all deletion jobs for a key so a test never leaves
// backlog behind.
func deleteDeletionJobs(t *testing.T, db *pgxpool.Pool, key string) {
	t.Helper()
	_, _ = db.Exec(context.Background(), `DELETE FROM media_deletion_jobs WHERE storage_key = $1`, key)
}

// activeJobCount returns the number of not-yet-completed deletion jobs for a key.
func activeJobCount(t *testing.T, db *pgxpool.Pool, key string) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM media_deletion_jobs WHERE storage_key = $1 AND completed_at IS NULL`, key).Scan(&count))
	return count
}

// TestRetireRetainedMediaConcurrentAtomic starts two cleanup operations against
// the same active media record simultaneously (channel barrier, no sleeps) and
// asserts both finish without an unhandled conflict, the media is removed and its
// key cleared, and exactly one active deletion job remains for the original key.
func TestRetireRetainedMediaConcurrentAtomic(t *testing.T) {
	db := testDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	photoID, storageKey := seedRetentionPhoto(t, db)
	t.Cleanup(func() { deleteDeletionJobs(t, db, storageKey) })

	// Deterministic barrier: both goroutines block on the channel and are
	// released together when it is closed.
	barrier := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	for i := range errs {
		go func(i int) {
			defer wg.Done()
			<-barrier
			errs[i] = repository.RetireRetainedMedia(ctx, photoID)
		}(i)
	}
	close(barrier)
	wg.Wait()

	for i, err := range errs {
		require.NoErrorf(t, err, "concurrent retire %d returned an error", i)
	}

	var lifecycleStatus, storageKeyAfter string
	require.NoError(t, db.QueryRow(ctx,
		`SELECT lifecycle_status, COALESCE(storage_key, '') FROM photos WHERE id = $1`, photoID).
		Scan(&lifecycleStatus, &storageKeyAfter))
	require.Equal(t, "removed", lifecycleStatus, "media must be marked removed")
	require.Empty(t, storageKeyAfter, "storage key must be cleared")

	require.Equal(t, 1, activeJobCount(t, db, storageKey), "exactly one active deletion job is expected")
}

// TestRetireRetainedMediaSucceedsWithExistingObligation proves that when a
// deletion obligation for the key already exists, the atomic cleanup still marks
// the media removed without creating a duplicate active job.
func TestRetireRetainedMediaSucceedsWithExistingObligation(t *testing.T) {
	db := testDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	photoID, storageKey := seedRetentionPhoto(t, db)
	t.Cleanup(func() { deleteDeletionJobs(t, db, storageKey) })

	// Simulate a pre-existing active obligation (e.g. enqueued by account/group
	// deletion). next_attempt_at is pushed into the future so the running backend
	// never claims it during the test.
	_, err := db.Exec(ctx,
		`INSERT INTO media_deletion_jobs (id, storage_key, source, next_attempt_at) VALUES ($1, $2, 'account', CURRENT_TIMESTAMP + interval '1 hour')`,
		"existing-job-"+unique("x"), storageKey)
	require.NoError(t, err)

	require.NoError(t, repository.RetireRetainedMedia(ctx, photoID), "retire must succeed when an obligation exists")

	var lifecycleStatus, storageKeyAfter string
	require.NoError(t, db.QueryRow(ctx,
		`SELECT lifecycle_status, COALESCE(storage_key, '') FROM photos WHERE id = $1`, photoID).
		Scan(&lifecycleStatus, &storageKeyAfter))
	require.Equal(t, "removed", lifecycleStatus)
	require.Empty(t, storageKeyAfter)

	require.Equal(t, 1, activeJobCount(t, db, storageKey), "no duplicate active job should be created")
}

// TestCompletedDeletionJobAllowsNewActiveJob proves a completed job for a key
// does not prevent a new active job for the same key, because the partial unique
// index only covers completed_at IS NULL rows.
func TestCompletedDeletionJobAllowsNewActiveJob(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	key := "completed-repro/" + unique("k")
	t.Cleanup(func() { deleteDeletionJobs(t, db, key) })

	// A job that has already completed.
	_, err := db.Exec(ctx,
		`INSERT INTO media_deletion_jobs (id, storage_key, source, next_attempt_at, completed_at) VALUES ($1, $2, 'retention', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"done-job-"+unique("x"), key)
	require.NoError(t, err)

	// Enqueuing a new active job for the same key must succeed.
	require.NoError(t, repository.EnqueueMediaDeletion(ctx, "manual", []string{key}))

	require.Equal(t, 1, activeJobCount(t, db, key), "a new active job is allowed after completion")
	var completedCount int
	require.NoError(t, db.QueryRow(ctx,
		`SELECT COUNT(*) FROM media_deletion_jobs WHERE storage_key = $1 AND completed_at IS NOT NULL`, key).Scan(&completedCount))
	require.Equal(t, 1, completedCount)
}

// TestPartialUniqueIndexRejectsDuplicateActiveJobs is the migration assertion:
// it proves the partial unique index exists (unique, partial on
// completed_at IS NULL) and that the database itself rejects two active jobs for
// the same storage key.
func TestPartialUniqueIndexRejectsDuplicateActiveJobs(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Structural assertion: the index exists, is unique, and is partial.
	var indexDef string
	require.NoError(t, db.QueryRow(ctx,
		`SELECT indexdef FROM pg_indexes WHERE indexname = 'media_deletion_jobs_active_storage_key_idx'`).Scan(&indexDef))
	require.Contains(t, indexDef, "UNIQUE INDEX media_deletion_jobs_active_storage_key_idx")
	require.Contains(t, indexDef, "WHERE (completed_at IS NULL)")

	key := "dup-reject/" + unique("k")
	t.Cleanup(func() { deleteDeletionJobs(t, db, key) })

	_, err := db.Exec(ctx,
		`INSERT INTO media_deletion_jobs (id, storage_key, source) VALUES ($1, $2, 'manual')`,
		"dup-1-"+unique("x"), key)
	require.NoError(t, err)

	// A second active job for the same key must violate the partial unique index.
	_, err = db.Exec(ctx,
		`INSERT INTO media_deletion_jobs (id, storage_key, source) VALUES ($1, $2, 'manual')`,
		"dup-2-"+unique("x"), key)
	require.Error(t, err, "second active job should be rejected")
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
	require.Equal(t, "23505", pgErr.Code, "expected a unique violation from the partial unique index")
}
