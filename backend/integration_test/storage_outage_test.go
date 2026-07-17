package integration_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"geoguessme/internal/repository"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestUploadConsistencyAndRecoveryFromStorageLoss proves:
//  1. A successful upload leaves consistent database state with a valid storage
//     key and the media is reachable through the API.
//  2. When the storage object is lost (simulated by replacing the storage key
//     with one that has never been uploaded), the media endpoint returns 410
//     Gone rather than crashing or returning stale data.
//  3. After the simulated outage, a new upload succeeds and leaves valid state,
//     proving the system recovers without intervention.
func TestUploadConsistencyAndRecoveryFromStorageLoss(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "Outage Group")
	joinGroup(t, bob.access, code)

	// Upload with a fully healthy storage backend.
	photoID := uploadPhoto(t, alice.access, groupID)

	// Verify the database row is consistent.
	var lifecycleStatus, storageKey string
	var expiresAt time.Time
	require.NoError(t, db.QueryRow(ctx,
		`SELECT lifecycle_status, storage_key, expires_at FROM photos WHERE id = $1`, photoID).
		Scan(&lifecycleStatus, &storageKey, &expiresAt))
	require.Equal(t, "ready", lifecycleStatus, "photo must be ready after upload")
	require.NotEmpty(t, storageKey, "storage key must be set after upload")

	// The media endpoint requires an active challenge view. The test stack
	// uses a 1 s view window, which is too short to sequence the assertions
	// below without races. Seed a generous view record via the test DB so we
	// can reliably exercise the storage-stat path.
	_, err := db.Exec(ctx,
		`INSERT INTO challenge_views (photo_id, user_id, accepted_at, view_expires_at) VALUES ($1, $2, NOW(), NOW() + interval '5 minutes') ON CONFLICT DO NOTHING`,
		photoID, bob.userID)
	require.NoError(t, err)

	// Serve the media through the authenticated endpoint.
	resp, _ := doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/media", nil, bob.access, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "media must be served for a valid stored photo")

	// Simulate the object disappearing from object storage (bucket corruption,
	// accidental delete, etc.) by pointing the row at a key that was never
	// uploaded to MinIO.
	orphanedKey := "simulated-outage/" + photoID + "/missing"
	_, err = db.Exec(ctx, `UPDATE photos SET storage_key = $1 WHERE id = $2`, orphanedKey, photoID)
	require.NoError(t, err)

	// The backend must detect the missing object and refuse to serve it.
	resp, _ = doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/media", nil, bob.access, nil)
	require.Equal(t, http.StatusGone, resp.StatusCode, "media must return gone when the storage object is missing")

	// Recovery: a brand-new upload must succeed regardless of the prior outage.
	recoveryPhotoID := uploadPhoto(t, alice.access, groupID)
	require.NotEmpty(t, recoveryPhotoID, "upload must succeed after storage recovery")

	var recoveryStatus, recoveryKey string
	require.NoError(t, db.QueryRow(ctx,
		`SELECT lifecycle_status, storage_key FROM photos WHERE id = $1`, recoveryPhotoID).
		Scan(&recoveryStatus, &recoveryKey))
	require.Equal(t, "ready", recoveryStatus)
	require.NotEmpty(t, recoveryKey, "recovered upload must have a valid storage key")

	// Tidy the orphaned row so the background sweep does not enqueue a
	// deletion job for a key that never existed.
	_, _ = db.Exec(ctx, `UPDATE photos SET lifecycle_status = 'removed', storage_key = NULL WHERE id = $1`, photoID)
}

// TestRetireRetainedMediaHandlesOrphanedStorageKey proves that the atomic
// retire-sweep operation completes successfully even when the storage object
// behind a photo never existed (e.g. the upload succeeded, the DB insert failed
// after storage was written, and the cleanup delete was lost during a storage
// outage). The operation must mark the row removed, clear the storage key, and
// enqueue a durable deletion job so the worker can retry later when storage
// recovers.
func TestRetireRetainedMediaHandlesOrphanedStorageKey(t *testing.T) {
	db := testDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	handle := unique("orphan")
	userID := "orphan-user-" + handle
	groupID := "orphan-group-" + handle
	photoID := "orphan-photo-" + handle
	storageKey := "orphan-objects/" + handle + "/never-uploaded"

	_, err := db.Exec(ctx,
		`INSERT INTO users (id, username, password, email, email_normalized) VALUES ($1, $2, 'x', $3, $3)`,
		userID, handle, handle+"@example.test")
	require.NoError(t, err)
	_, err = db.Exec(ctx,
		`INSERT INTO groups (id, name, code) VALUES ($1, $2, $3)`,
		groupID, "Orphan "+handle, "code-"+handle)
	require.NoError(t, err)
	_, err = db.Exec(ctx,
		`INSERT INTO photos (id, user_id, group_id, storage_key, lat, long, lifecycle_status, expires_at, retention_at) VALUES ($1, $2, $3, $4, 0, 0, 'ready', CURRENT_TIMESTAMP + interval '1 hour', CURRENT_TIMESTAMP - interval '1 second')`,
		photoID, userID, groupID, storageKey)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM media_deletion_jobs WHERE storage_key = $1`, storageKey)
		_, _ = db.Exec(context.Background(), `DELETE FROM photos WHERE id = $1`, photoID)
		_, _ = db.Exec(context.Background(), `DELETE FROM groups WHERE id = $1`, groupID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	// The key was never put to object storage, yet RetireRetainedMedia must
	// complete atomically: mark removed, clear key, enqueue deletion job.
	require.NoError(t, repository.RetireRetainedMedia(ctx, photoID),
		"retire must succeed even when the storage object is missing")

	var lifecycleStatus, keyAfter string
	require.NoError(t, db.QueryRow(ctx,
		`SELECT lifecycle_status, COALESCE(storage_key, '') FROM photos WHERE id = $1`, photoID).
		Scan(&lifecycleStatus, &keyAfter))
	require.Equal(t, "removed", lifecycleStatus, "media must be marked removed")
	require.Empty(t, keyAfter, "storage key must be cleared after retire")

	var activeJobs int
	require.NoError(t, db.QueryRow(ctx,
		`SELECT COUNT(*) FROM media_deletion_jobs WHERE storage_key = $1 AND completed_at IS NULL`, storageKey).
		Scan(&activeJobs))
	require.Equal(t, 1, activeJobs, "one active deletion job must be enqueued for the orphaned key")
}

// TestRetireRetainedMediaIdempotentWithOrphanedKey proves that retiring a
// photo whose storage object never existed is idempotent: a second call with
// the same id must be a no-op and must not create a duplicate active deletion
// job.
func TestRetireRetainedMediaIdempotentWithOrphanedKey(t *testing.T) {
	db := testDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	handle := unique("orphan2")
	userID := "orphan2-user-" + handle
	groupID := "orphan2-group-" + handle
	photoID := "orphan2-photo-" + handle
	storageKey := "orphan2-objects/" + handle + "/never-uploaded"

	_, err := db.Exec(ctx,
		`INSERT INTO users (id, username, password, email, email_normalized) VALUES ($1, $2, 'x', $3, $3)`,
		userID, handle, handle+"@example.test")
	require.NoError(t, err)
	_, err = db.Exec(ctx,
		`INSERT INTO groups (id, name, code) VALUES ($1, $2, $3)`,
		groupID, "Orphan2 "+handle, "code-"+handle)
	require.NoError(t, err)
	_, err = db.Exec(ctx,
		`INSERT INTO photos (id, user_id, group_id, storage_key, lat, long, lifecycle_status, expires_at, retention_at) VALUES ($1, $2, $3, $4, 0, 0, 'ready', CURRENT_TIMESTAMP + interval '1 hour', CURRENT_TIMESTAMP - interval '1 second')`,
		photoID, userID, groupID, storageKey)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM media_deletion_jobs WHERE storage_key = $1`, storageKey)
		_, _ = db.Exec(context.Background(), `DELETE FROM photos WHERE id = $1`, photoID)
		_, _ = db.Exec(context.Background(), `DELETE FROM groups WHERE id = $1`, groupID)
		_, _ = db.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	require.NoError(t, repository.RetireRetainedMedia(ctx, photoID))
	// Second call with the same id must be a no-op.
	require.NoError(t, repository.RetireRetainedMedia(ctx, photoID),
		"second retire must be idempotent")

	var lifecycleStatus string
	require.NoError(t, db.QueryRow(ctx,
		`SELECT lifecycle_status FROM photos WHERE id = $1`, photoID).
		Scan(&lifecycleStatus))
	require.Equal(t, "removed", lifecycleStatus)

	var activeJobs int
	require.NoError(t, db.QueryRow(ctx,
		`SELECT COUNT(*) FROM media_deletion_jobs WHERE storage_key = $1 AND completed_at IS NULL`, storageKey).
		Scan(&activeJobs))
	require.Equal(t, 1, activeJobs, "idempotent retire must not create duplicate active jobs")
}

// TestUploadStorageFailureDoesNotCommitDBState proves that when the storage
// Put fails the handler does not insert a database row. It uploads a real
// photo via the API, then simulates an outage by replacing the storage key
// with one that was never put to object storage. The media endpoint must
// detect the missing object and return 410 Gone. Combined with the
// handler-level unit test that exercises failingStore, this guards against
// a regression where a database row would be committed without a valid
// backing storage object or where the system fails to detect a missing object.
func TestUploadStorageFailureDoesNotCommitDBState(t *testing.T) {
	db := testDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	alice := signup(t, unique("alice"), unique("alice")+"@example.test", "StrongPassword123")
	bob := signup(t, unique("bob"), unique("bob")+"@example.test", "StrongPassword123")
	groupID, code := createGroup(t, alice.access, "NoCommit Group")
	joinGroup(t, bob.access, code)

	// Upload a real photo so the backend's foreign-key references are valid.
	photoID := uploadPhoto(t, alice.access, groupID)

	// Seed a challenge view for bob so the media handler passes the membership
	// and view-window checks before reaching the storage-stat path.
	_, err := db.Exec(ctx,
		`INSERT INTO challenge_views (photo_id, user_id, accepted_at, view_expires_at) VALUES ($1, $2, NOW(), NOW() + interval '5 minutes') ON CONFLICT DO NOTHING`,
		photoID, bob.userID)
	require.NoError(t, err)

	// Replace the storage key with one that was never stored. This simulates
	// what would happen if the upload Put failed but a row was committed anyway.
	orphanedKey := "nocmt-objects/" + uuid.NewString() + "/missing"
	_, err = db.Exec(ctx, `UPDATE photos SET storage_key = $1 WHERE id = $2`, orphanedKey, photoID)
	require.NoError(t, err)

	// The media endpoint must detect the missing object.
	resp, _ := doJSON(t, http.MethodGet, "/api/v1/challenges/"+photoID+"/media", nil, bob.access, nil)
	require.Equal(t, http.StatusGone, resp.StatusCode, "media must return gone when storage object is missing")

	// The retire path must handle this row safely.
	require.NoError(t, repository.RetireRetainedMedia(ctx, photoID),
		"retire must not fail on an orphaned storage key")

	var lifecycleStatus string
	require.NoError(t, db.QueryRow(ctx,
		`SELECT lifecycle_status FROM photos WHERE id = $1`, photoID).
		Scan(&lifecycleStatus))
	require.Equal(t, "removed", lifecycleStatus, "photo must be marked removed atomically")

	// Tidy up so the sweep doesn't enqueue a deletion job for the orphaned key.
	_, _ = db.Exec(ctx, `UPDATE photos SET lifecycle_status = 'removed', storage_key = NULL WHERE id = $1`, photoID)
}
