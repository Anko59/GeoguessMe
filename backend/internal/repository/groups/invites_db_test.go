package groups

import (
	"context"
	"errors"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/pashagolub/pgxmock/v4"
)

func TestInviteByTokenHashAndByID(t *testing.T) {
	ctx := context.Background()
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC()
	invite := &models.GroupInvite{
		ID: "invite-1", GroupID: "group-1", CreatorUserID: "user-1", TokenHash: "hash",
		CreatedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour),
	}
	columns := []string{"id", "group_id", "creator_user_id", "token_hash", "created_at", "expires_at", "revoked_at"}
	row := func() *pgxmock.Rows {
		return pgxmock.NewRows(columns).AddRow(invite.ID, invite.GroupID, invite.CreatorUserID, invite.TokenHash, invite.CreatedAt, invite.ExpiresAt, nil)
	}

	mock.ExpectQuery("SELECT id, group_id, creator_user_id, token_hash, created_at, expires_at, revoked_at FROM group_invites WHERE token_hash").WithArgs("hash").WillReturnRows(row())
	got, err := repo.InviteByTokenHash(ctx, "hash")
	if err != nil || got == nil || got.ID != "invite-1" || got.TokenHash != "hash" {
		t.Fatalf("InviteByTokenHash = %+v, %v", got, err)
	}

	mock.ExpectQuery("SELECT id, group_id, creator_user_id, token_hash, created_at, expires_at, revoked_at FROM group_invites WHERE id").WithArgs("invite-1").WillReturnRows(row())
	got, err = repo.InviteByID(ctx, "invite-1")
	if err != nil || got == nil || got.GroupID != "group-1" {
		t.Fatalf("InviteByID = %+v, %v", got, err)
	}

	mock.ExpectQuery("SELECT id, group_id, creator_user_id, token_hash, created_at, expires_at, revoked_at FROM group_invites WHERE token_hash").WithArgs("missing").WillReturnRows(pgxmock.NewRows(columns))
	got, err = repo.InviteByTokenHash(ctx, "missing")
	if err != nil || got != nil {
		t.Fatalf("missing InviteByTokenHash = %+v, %v", got, err)
	}
}

func TestCreateInviteEnforcesCaps(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("inserts when under caps", func(t *testing.T) {
		mock := newMockPool(t)
		repo := NewRepository(mock)
		invite := &models.GroupInvite{ID: "invite-1", GroupID: "group-1", CreatorUserID: "user-1", TokenHash: "hash", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
		mock.ExpectBegin()
		mock.ExpectExec("SELECT 1 FROM groups").WithArgs("group-1").WillReturnResult(pgxmock.NewResult("SELECT", 1))
		mock.ExpectExec("SELECT 1 FROM users").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("SELECT", 1))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM group_invites WHERE group_id").WithArgs("group-1").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM group_invites WHERE creator_user_id").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectExec("INSERT INTO group_invites").WithArgs("invite-1", "group-1", "user-1", "hash", now, now.Add(time.Hour)).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()
		if err := repo.CreateInvite(ctx, invite, 5, 10); err != nil {
			t.Fatalf("CreateInvite = %v", err)
		}
	})

	t.Run("rejects at group active cap", func(t *testing.T) {
		mock := newMockPool(t)
		repo := NewRepository(mock)
		invite := &models.GroupInvite{ID: "invite-2", GroupID: "group-1", CreatorUserID: "user-1", TokenHash: "hash", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
		mock.ExpectBegin()
		mock.ExpectExec("SELECT 1 FROM groups").WithArgs("group-1").WillReturnResult(pgxmock.NewResult("SELECT", 1))
		mock.ExpectExec("SELECT 1 FROM users").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("SELECT", 1))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM group_invites WHERE group_id").WithArgs("group-1").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(5))
		if err := repo.CreateInvite(ctx, invite, 5, 10); !errors.Is(err, ErrTooManyGroupInvites) {
			t.Fatalf("CreateInvite at group cap = %v, want ErrTooManyGroupInvites", err)
		}
	})

	t.Run("rejects at user daily cap", func(t *testing.T) {
		mock := newMockPool(t)
		repo := NewRepository(mock)
		invite := &models.GroupInvite{ID: "invite-3", GroupID: "group-1", CreatorUserID: "user-1", TokenHash: "hash", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
		mock.ExpectBegin()
		mock.ExpectExec("SELECT 1 FROM groups").WithArgs("group-1").WillReturnResult(pgxmock.NewResult("SELECT", 1))
		mock.ExpectExec("SELECT 1 FROM users").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("SELECT", 1))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM group_invites WHERE group_id").WithArgs("group-1").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM group_invites WHERE creator_user_id").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(10))
		if err := repo.CreateInvite(ctx, invite, 5, 10); !errors.Is(err, ErrTooManyUserInvites) {
			t.Fatalf("CreateInvite at user cap = %v, want ErrTooManyUserInvites", err)
		}
	})
}

func TestListInvitesByGroupAndRevoke(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	mock := newMockPool(t)
	repo := NewRepository(mock)

	mock.ExpectQuery("SELECT id, group_id, creator_user_id, token_hash, created_at, expires_at, revoked_at FROM group_invites WHERE group_id").WithArgs("group-1").
		WillReturnRows(pgxmock.NewRows([]string{"id", "group_id", "creator_user_id", "token_hash", "created_at", "expires_at", "revoked_at"}).
			AddRow("invite-1", "group-1", "user-1", "hash-1", now, now.Add(time.Hour), nil).
			AddRow("invite-2", "group-1", "user-1", "hash-2", now, now.Add(time.Hour), nil))
	invites, err := repo.ListInvitesByGroup(ctx, "group-1")
	if err != nil || len(invites) != 2 || invites[0].TokenHash != "hash-1" {
		t.Fatalf("ListInvitesByGroup = %+v, %v", invites, err)
	}

	// Revoke an active invite returns true; re-revoke or missing returns false.
	mock.ExpectExec("UPDATE group_invites SET revoked_at = now\\(\\) WHERE id = \\$1 AND group_id = \\$2 AND revoked_at IS NULL").WithArgs("invite-1", "group-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	revoked, err := repo.RevokeInvite(ctx, "invite-1", "group-1")
	if err != nil || !revoked {
		t.Fatalf("RevokeInvite active = %v, %v", revoked, err)
	}

	mock.ExpectExec("UPDATE group_invites SET revoked_at = now\\(\\) WHERE id = \\$1 AND group_id = \\$2 AND revoked_at IS NULL").WithArgs("invite-9", "group-1").WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	revoked, err = repo.RevokeInvite(ctx, "invite-9", "group-1")
	if err != nil || revoked {
		t.Fatalf("RevokeInvite missing = %v, %v", revoked, err)
	}
}

func TestGroupPreview(t *testing.T) {
	ctx := context.Background()
	mock := newMockPool(t)
	repo := NewRepository(mock)

	mock.ExpectQuery("SELECT g.name, \\(SELECT COUNT\\(\\*\\) FROM group_members gm WHERE gm.group_id = g.id\\)").WithArgs("group-1").
		WillReturnRows(pgxmock.NewRows([]string{"name", "count"}).AddRow("Paris", 3))
	name, count, err := repo.GroupPreview(ctx, "group-1")
	if err != nil || name != "Paris" || count != 3 {
		t.Fatalf("GroupPreview = %q, %d, %v", name, count, err)
	}
}
