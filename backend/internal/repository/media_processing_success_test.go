package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/pashagolub/pgxmock/v4"
)

// TestCompleteChallengeProcessing proves the atomic success path: the per-group
// photo rows and the job-completion UPDATE commit in one transaction, and the
// job's canonical key and result reference the first photo.
func TestCompleteChallengeProcessing(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	photos := []*models.Photo{
		{ID: "photo-1", UserID: "user-1", GroupID: "g1", StorageKey: "photos/canonical-1", MIMEType: "video/mp4", ByteSize: 12, LifecycleStatus: "ready", CreatedAt: now, ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)},
		{ID: "photo-2", UserID: "user-1", GroupID: "g2", StorageKey: "photos/canonical-2", MIMEType: "video/mp4", ByteSize: 12, LifecycleStatus: "ready", CreatedAt: now, ExpiresAt: now.Add(time.Hour), RetentionAt: now.Add(24 * time.Hour)},
	}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT NOT EXISTS").WithArgs("user-1", []string{"g1", "g2"}).WillReturnRows(pgxmock.NewRows([]string{"authorized"}).AddRow(true))
	for _, photo := range photos {
		mock.ExpectExec("INSERT INTO photos").WithArgs(photo.ID, photo.UserID, photo.GroupID, photo.URL, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.Lat, photo.Long, photo.LifecycleStatus, photo.HideLocation, photo.CreatedAt, photo.ExpiresAt, photo.RetentionAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	}
	mock.ExpectExec("UPDATE media_processing_jobs.*status = 'ready'").WithArgs("job-1", "photos/canonical-1", "photo", "photo-1", "video/mp4", int64(12), "lease-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").WithArgs(pgxmock.AnyArg(), "quarantine/job-1", "media-processing").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	if err := repo.CompleteChallengeProcessing(ctx, "job-1", "lease-1", "quarantine/job-1", photos, "video/mp4", 12); err != nil {
		t.Fatalf("CompleteChallengeProcessing: %v", err)
	}
}

// TestCompleteChallengeProcessingRollsBackOnPhotoInsertFailure proves a photo
// insert failure aborts the transaction: no job completion UPDATE and no commit
// occur, so the job is never marked ready without its records.
func TestCompleteChallengeProcessingRollsBackOnPhotoInsertFailure(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	ctx := context.Background()
	photoErr := errors.New("photo insert failed")
	photo := &models.Photo{ID: "photo-1", UserID: "user-1", GroupID: "g1", StorageKey: "photos/canonical-1", MIMEType: "video/mp4", ByteSize: 12}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT NOT EXISTS").WithArgs("user-1", []string{"g1"}).WillReturnRows(pgxmock.NewRows([]string{"authorized"}).AddRow(true))
	mock.ExpectExec("INSERT INTO photos").WithArgs(photo.ID, photo.UserID, photo.GroupID, photo.URL, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.Lat, photo.Long, photo.LifecycleStatus, photo.HideLocation, photo.CreatedAt, photo.ExpiresAt, photo.RetentionAt).WillReturnError(photoErr)
	mock.ExpectRollback()

	if err := repo.CompleteChallengeProcessing(ctx, "job-1", "lease-1", "quarantine/job-1", []*models.Photo{photo}, "video/mp4", 12); !errors.Is(err, photoErr) {
		t.Fatalf("expected photo insert error to propagate, got %v", err)
	}
}

// TestCompleteChallengeProcessingRejectsEmptyPhotos proves the guard that a
// challenge job must produce at least one photo before a transaction opens.
func TestCompleteChallengeProcessingRejectsEmptyPhotos(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	if err := repo.CompleteChallengeProcessing(context.Background(), "job-1", "lease-1", "quarantine/job-1", nil, "video/mp4", 0); err == nil {
		t.Fatal("expected an error for an empty photo slice")
	}
}

func TestCompleteChallengeProcessingRechecksMembership(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	photo := &models.Photo{ID: "photo-1", UserID: "user-1", GroupID: "g1"}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT NOT EXISTS").WithArgs("user-1", []string{"g1"}).WillReturnRows(pgxmock.NewRows([]string{"authorized"}).AddRow(false))
	mock.ExpectRollback()
	err := repo.CompleteChallengeProcessing(context.Background(), "job-1", "lease-1", "quarantine/job-1", []*models.Photo{photo}, "video/mp4", 12)
	if !errors.Is(err, models.ErrMediaProcessingAuthorizationRevoked) {
		t.Fatalf("membership revocation error = %v", err)
	}
}

// TestCompleteChatProcessing proves the atomic chat success path: the
// attachment, the message, and the job-completion UPDATE commit in one
// transaction, and the message carries the media reference afterwards.
func TestCompleteChatProcessing(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	replyTo := "msg-0"
	asset := &models.ChatMedia{ID: "asset-1", GroupID: "g1", UserID: "user-1", StorageKey: "chat-media/canonical-1", MIMEType: "video/mp4", ByteSize: 12, CreatedAt: now}
	msg := &models.Message{ID: "msg-1", GroupID: "g1", UserID: "user-1", ReplyToID: &replyTo, Content: "check this", CreatedAt: now}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS.*group_members").WithArgs("g1", "user-1").WillReturnRows(pgxmock.NewRows([]string{"authorized"}).AddRow(true))
	mock.ExpectQuery("SELECT username, avatar FROM users").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"username", "avatar"}).AddRow("alice", "avatar-1"))
	mock.ExpectQuery("SELECT EXISTS").WithArgs("msg-0", "g1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectExec("INSERT INTO chat_media").WithArgs(asset.ID, asset.GroupID, asset.UserID, asset.StorageKey, asset.MIMEType, asset.ByteSize, asset.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO messages").WithArgs(msg.ID, msg.GroupID, msg.UserID, asset.ID, msg.ReplyToID, msg.Content, msg.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("UPDATE media_processing_jobs.*status = 'ready'").WithArgs("job-2", "chat-media/canonical-1", "chat", "msg-1", "video/mp4", int64(12), "lease-2").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec("INSERT INTO media_deletion_jobs").WithArgs(pgxmock.AnyArg(), "quarantine/job-2", "media-processing").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()

	if err := repo.CompleteChatProcessing(ctx, "job-2", "lease-2", "quarantine/job-2", msg, asset); err != nil {
		t.Fatalf("CompleteChatProcessing: %v", err)
	}
	if msg.Kind != "media" || msg.MediaID == nil || *msg.MediaID != asset.ID || msg.MediaType != "video/mp4" {
		t.Fatalf("message media reference not set: %+v", msg)
	}
}

// TestCompleteChatProcessingRollsBackOnJobUpdateFailure proves a failure after
// the message insert aborts everything: the rollback is explicit and no commit
// occurs, so a job is never ready without its message nor a message without its
// ready job.
func TestCompleteChatProcessingRollsBackOnJobUpdateFailure(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	ctx := context.Background()
	updateErr := errors.New("job update failed")
	now := time.Now().UTC().Truncate(time.Microsecond)
	asset := &models.ChatMedia{ID: "asset-1", GroupID: "g1", UserID: "user-1", StorageKey: "chat-media/canonical-1", MIMEType: "video/mp4", ByteSize: 12, CreatedAt: now}
	msg := &models.Message{ID: "msg-1", GroupID: "g1", UserID: "user-1", Content: "check this", CreatedAt: now}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT EXISTS.*group_members").WithArgs("g1", "user-1").WillReturnRows(pgxmock.NewRows([]string{"authorized"}).AddRow(true))
	mock.ExpectQuery("SELECT username, avatar FROM users").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"username", "avatar"}).AddRow("alice", "avatar-1"))
	mock.ExpectExec("INSERT INTO chat_media").WithArgs(asset.ID, asset.GroupID, asset.UserID, asset.StorageKey, asset.MIMEType, asset.ByteSize, asset.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO messages").WithArgs(msg.ID, msg.GroupID, msg.UserID, asset.ID, msg.ReplyToID, msg.Content, msg.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("UPDATE media_processing_jobs.*status = 'ready'").WithArgs("job-2", "chat-media/canonical-1", "chat", "msg-1", "video/mp4", int64(12), "lease-2").WillReturnError(updateErr)
	mock.ExpectRollback()

	if err := repo.CompleteChatProcessing(ctx, "job-2", "lease-2", "quarantine/job-2", msg, asset); !errors.Is(err, updateErr) {
		t.Fatalf("expected job update error to propagate, got %v", err)
	}
}

// TestCompleteProcessingJobTx proves the tx-scoped completion UPDATE runs
// against the supplied transaction.
func TestCompleteProcessingJobTx(t *testing.T) {
	mock := newMockPool(t)
	repo := NewRepository(mock)
	ctx := context.Background()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE media_processing_jobs.*status = 'ready'").WithArgs("job-1", "photos/canonical-1", "photo", "photo-1", "video/mp4", int64(12), "lease-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectCommit()
	tx, err := mock.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := repo.CompleteProcessingJobTx(ctx, tx, "job-1", "lease-1", "photos/canonical-1", "photo", "photo-1", "video/mp4", 12); err != nil {
		t.Fatalf("CompleteProcessingJobTx: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
