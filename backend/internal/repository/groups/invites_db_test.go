package groups

import (
	"context"
	"errors"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/pashagolub/pgxmock/v4"
)

func TestInviteByID(t *testing.T) {
	ctx := context.Background()
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC()
	invite := &models.GroupInvite{
		ID: "invite-1", GroupID: "group-1", CreatorUserID: "user-1", TokenHash: "hash",
		CreatedAt: now, ExpiresAt: now.Add(7 * 24 * time.Hour),
	}
	metadataColumns := []string{"id", "group_id", "creator_user_id", "created_at", "expires_at", "revoked_at"}

	mock.ExpectQuery("SELECT id, group_id, creator_user_id, created_at, expires_at, revoked_at FROM group_invites WHERE id").WithArgs("invite-1").
		WillReturnRows(pgxmock.NewRows(metadataColumns).AddRow(invite.ID, invite.GroupID, invite.CreatorUserID, invite.CreatedAt, invite.ExpiresAt, nil))
	got, err := repo.InviteByID(ctx, "invite-1")
	if err != nil || got == nil || got.GroupID != "group-1" || got.TokenHash != "" {
		t.Fatalf("InviteByID = %+v, %v", got, err)
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
		mock.ExpectQuery("SELECT 1 FROM group_members").WithArgs("group-1", "user-1").WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1))
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
		mock.ExpectQuery("SELECT 1 FROM group_members").WithArgs("group-1", "user-1").WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1))
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
		mock.ExpectQuery("SELECT 1 FROM group_members").WithArgs("group-1", "user-1").WillReturnRows(pgxmock.NewRows([]string{"one"}).AddRow(1))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM group_invites WHERE group_id").WithArgs("group-1").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM group_invites WHERE creator_user_id").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(10))
		if err := repo.CreateInvite(ctx, invite, 5, 10); !errors.Is(err, ErrTooManyUserInvites) {
			t.Fatalf("CreateInvite at user cap = %v, want ErrTooManyUserInvites", err)
		}
	})
}

func TestCreateInviteRejectsMembershipRemovedInsideTransaction(t *testing.T) {
	ctx := context.Background()
	mock := newMockPool(t)
	repo := NewRepository(mock)
	now := time.Now().UTC()
	invite := &models.GroupInvite{ID: "invite-1", GroupID: "group-1", CreatorUserID: "user-1", TokenHash: "hash", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}

	mock.ExpectBegin()
	mock.ExpectExec("SELECT 1 FROM groups").WithArgs("group-1").WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectExec("SELECT 1 FROM users").WithArgs("user-1").WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("SELECT 1 FROM group_members").WithArgs("group-1", "user-1").WillReturnRows(pgxmock.NewRows([]string{"one"}))
	mock.ExpectRollback()
	if err := repo.CreateInvite(ctx, invite, 5, 10); !errors.Is(err, ErrNotMember) {
		t.Fatalf("CreateInvite after membership removal = %v, want ErrNotMember", err)
	}
}

func TestListInvitesByGroupAndRevoke(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	mock := newMockPool(t)
	repo := NewRepository(mock)

	mock.ExpectQuery("SELECT gi.id, gi.group_id, gi.creator_user_id, gi.created_at, gi.expires_at, gi.revoked_at").WithArgs("group-1", "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"id", "group_id", "creator_user_id", "created_at", "expires_at", "revoked_at"}).
			AddRow("invite-1", "group-1", "user-1", now, now.Add(time.Hour), nil).
			AddRow("invite-2", "group-1", "user-1", now, now.Add(time.Hour), nil))
	invites, err := repo.ListInvitesByGroup(ctx, "group-1", "user-1")
	if err != nil || len(invites) != 2 || invites[0].TokenHash != "" {
		t.Fatalf("ListInvitesByGroup = %+v, %v", invites, err)
	}

	// Revoke an active invite returns true; re-revoke or missing returns false.
	mock.ExpectExec("UPDATE group_invites gi SET revoked_at = now\\(\\)").WithArgs("invite-1", "group-1", "user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	revoked, err := repo.RevokeInvite(ctx, "invite-1", "group-1", "user-1")
	if err != nil || !revoked {
		t.Fatalf("RevokeInvite active = %v, %v", revoked, err)
	}

	mock.ExpectExec("UPDATE group_invites gi SET revoked_at = now\\(\\)").WithArgs("invite-9", "group-1", "user-1").WillReturnResult(pgxmock.NewResult("UPDATE", 0))
	revoked, err = repo.RevokeInvite(ctx, "invite-9", "group-1", "user-1")
	if err != nil || revoked {
		t.Fatalf("RevokeInvite missing = %v, %v", revoked, err)
	}
}

func TestJoinByInviteTokenHash(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("atomically joins a live invite", func(t *testing.T) {
		mock := newMockPool(t)
		repo := NewRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT g.id, g.name, g.code, g.created_at").WithArgs("hash", now).
			WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow("group-1", "Paris", "ABC123", now))
		mock.ExpectExec("INSERT INTO group_members").WithArgs("group-1", "user-1", now).WillReturnResult(pgxmock.NewResult("INSERT", 1))
		mock.ExpectCommit()
		group, err := repo.JoinByInviteTokenHash(ctx, "hash", "user-1", now)
		if err != nil || group == nil || group.ID != "group-1" {
			t.Fatalf("JoinByInviteTokenHash = %+v, %v", group, err)
		}
	})

	t.Run("rejects an invite that is not live", func(t *testing.T) {
		mock := newMockPool(t)
		repo := NewRepository(mock)
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT g.id, g.name, g.code, g.created_at").WithArgs("hash", now).
			WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}))
		mock.ExpectRollback()
		group, err := repo.JoinByInviteTokenHash(ctx, "hash", "user-1", now)
		if err != nil || group != nil {
			t.Fatalf("missing JoinByInviteTokenHash = %+v, %v", group, err)
		}
	})
}

func TestGroupPreviewByInviteTokenHash(t *testing.T) {
	ctx := context.Background()
	mock := newMockPool(t)
	repo := NewRepository(mock)

	now := time.Now().UTC()
	mock.ExpectQuery("SELECT g.name, \\(SELECT COUNT\\(\\*\\) FROM group_members gm WHERE gm.group_id = g.id\\)").WithArgs("hash", now).
		WillReturnRows(pgxmock.NewRows([]string{"name", "count"}).AddRow("Paris", 3))
	name, count, found, err := repo.GroupPreviewByInviteTokenHash(ctx, "hash", now)
	if err != nil || !found || name != "Paris" || count != 3 {
		t.Fatalf("GroupPreviewByInviteTokenHash = %q, %d, %v, %v", name, count, found, err)
	}

	mock.ExpectQuery("SELECT g.name, \\(SELECT COUNT\\(\\*\\) FROM group_members gm WHERE gm.group_id = g.id\\)").WithArgs("missing", now).
		WillReturnRows(pgxmock.NewRows([]string{"name", "count"}))
	name, count, found, err = repo.GroupPreviewByInviteTokenHash(ctx, "missing", now)
	if err != nil || found || name != "" || count != 0 {
		t.Fatalf("missing GroupPreviewByInviteTokenHash = %q, %d, %v, %v", name, count, found, err)
	}
}
