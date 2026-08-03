package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
)

func TestGroupPhotoAndNotificationPersistence(t *testing.T) {
	mock := newMockPool(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	photo := &models.GroupPhoto{
		GroupID: "group-1", StorageKey: "groups/group-1/photo/new",
		MIMEType: "image/webp", ByteSize: 1234, CreatedAt: now,
	}

	mock.ExpectQuery("SELECT group_id, storage_key, mime_type").
		WithArgs(photo.GroupID).
		WillReturnRows(pgxmock.NewRows([]string{"group_id", "storage_key", "mime_type", "byte_size", "created_at"}).
			AddRow(photo.GroupID, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.CreatedAt))
	got, err := GetGroupPhotoContext(ctx, photo.GroupID)
	if err != nil || got == nil || *got != *photo {
		t.Fatalf("group photo = %+v, %v", got, err)
	}

	mock.ExpectQuery("SELECT group_id, storage_key, mime_type").
		WithArgs("missing").
		WillReturnError(pgx.ErrNoRows)
	got, err = GetGroupPhotoContext(ctx, "missing")
	if err != nil || got != nil {
		t.Fatalf("missing group photo = %+v, %v", got, err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT storage_key FROM group_photos").
		WithArgs(photo.GroupID).
		WillReturnRows(pgxmock.NewRows([]string{"storage_key"}).AddRow("groups/group-1/photo/old"))
	mock.ExpectExec("INSERT INTO group_photos").
		WithArgs(photo.GroupID, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.CreatedAt).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	previous, err := SetGroupPhotoContext(ctx, photo)
	if err != nil || previous != "groups/group-1/photo/old" {
		t.Fatalf("set group photo = %q, %v", previous, err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT storage_key FROM group_photos").
		WithArgs(photo.GroupID).
		WillReturnError(pgx.ErrNoRows)
	mock.ExpectExec("INSERT INTO group_photos").
		WithArgs(photo.GroupID, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.CreatedAt).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	previous, err = SetGroupPhotoContext(ctx, photo)
	if err != nil || previous != "" {
		t.Fatalf("create group photo = %q, %v", previous, err)
	}

	mock.ExpectQuery("SELECT COALESCE").
		WithArgs(photo.GroupID, "user-1").
		WillReturnRows(pgxmock.NewRows([]string{"enabled"}).AddRow(false))
	enabled, err := GetGroupNotificationPreferenceContext(ctx, photo.GroupID, "user-1")
	if err != nil || enabled {
		t.Fatalf("notification preference = %v, %v", enabled, err)
	}
	mock.ExpectExec("INSERT INTO group_notification_preferences").
		WithArgs(photo.GroupID, "user-1", true).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := SetGroupNotificationPreferenceContext(ctx, photo.GroupID, "user-1", true); err != nil {
		t.Fatalf("set notification preference: %v", err)
	}
}

func TestSetGroupPhotoRollsBackOnPersistenceFailure(t *testing.T) {
	mock := newMockPool(t)
	photo := &models.GroupPhoto{GroupID: "group-1", StorageKey: "new", MIMEType: "image/webp", ByteSize: 12, CreatedAt: time.Now()}
	persistErr := errors.New("write failed")
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT storage_key FROM group_photos").
		WithArgs(photo.GroupID).
		WillReturnRows(pgxmock.NewRows([]string{"storage_key"}).AddRow("old"))
	mock.ExpectExec("INSERT INTO group_photos").
		WithArgs(photo.GroupID, photo.StorageKey, photo.MIMEType, photo.ByteSize, photo.CreatedAt).
		WillReturnError(persistErr)
	mock.ExpectRollback()
	if _, err := SetGroupPhotoContext(context.Background(), photo); !errors.Is(err, persistErr) {
		t.Fatalf("set group photo error = %v", err)
	}
}

func TestGroupQueriesAndMembership(t *testing.T) {
	mock := newMockPool(t)
	now := time.Now().UTC()
	group := &models.Group{ID: "group-1", Name: "Paris", Code: "ABC123", CreatedAt: now}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO groups").WithArgs(group.ID, group.Name, group.Code, group.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectExec("INSERT INTO group_members").WithArgs(group.ID, "user-1", group.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	if err := CreateGroupAndMembership(context.Background(), group, "user-1"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO groups").WithArgs(group.ID, group.Name, group.Code, group.CreatedAt).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	if err := CreateGroup(group); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT id, name, code, created_at FROM groups WHERE code").WithArgs(group.Code).WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow(group.ID, group.Name, group.Code, group.CreatedAt))
	got, err := GetGroupByCodeContext(context.Background(), group.Code)
	if err != nil || got == nil || got.ID != group.ID {
		t.Fatalf("group by code = %+v, %v", got, err)
	}
	mock.ExpectQuery("SELECT id, name, code, created_at FROM groups WHERE code").WithArgs("missing").WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}))
	got, err = GetGroupByCode("missing")
	if err != nil || got != nil {
		t.Fatalf("missing group = %+v, %v", got, err)
	}
	mock.ExpectExec("INSERT INTO group_members").WithArgs(group.ID, "user-2", now).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := AddGroupMember(&models.GroupMember{GroupID: group.ID, UserID: "user-2", JoinedAt: now}); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("INSERT INTO group_members").WithArgs(group.ID, "user-3", now).WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := AddGroupMemberContext(context.Background(), &models.GroupMember{GroupID: group.ID, UserID: "user-3", JoinedAt: now}); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs(group.ID, "user-1").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	member, err := IsGroupMember(group.ID, "user-1")
	if err != nil || !member {
		t.Fatalf("membership = %v, %v", member, err)
	}
	mock.ExpectQuery("SELECT EXISTS").WithArgs(group.ID, "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	member, err = IsGroupMemberContext(context.Background(), group.ID, "user-2")
	if err != nil || member {
		t.Fatalf("non-membership = %v, %v", member, err)
	}
	mock.ExpectQuery("SELECT id, name, code, created_at FROM groups WHERE id").WithArgs(group.ID).WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow(group.ID, group.Name, group.Code, group.CreatedAt))
	got, err = GetGroupByID(group.ID)
	if err != nil || got == nil {
		t.Fatalf("group by id = %+v, %v", got, err)
	}
}

func TestSharesGroup(t *testing.T) {
	mock := newMockPool(t)
	// A user always shares a group with themself, without a query.
	shared, err := SharesGroupContext(context.Background(), "user-1", "user-1")
	if err != nil || !shared {
		t.Fatalf("self shared = %v, %v", shared, err)
	}
	// Two members of a common group share it.
	mock.ExpectQuery("SELECT EXISTS").WithArgs("user-1", "user-2").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(true))
	shared, err = SharesGroupContext(context.Background(), "user-1", "user-2")
	if err != nil || !shared {
		t.Fatalf("shared group = %v, %v", shared, err)
	}
	// Players in disjoint groups do not share one.
	mock.ExpectQuery("SELECT EXISTS").WithArgs("user-1", "user-3").WillReturnRows(pgxmock.NewRows([]string{"exists"}).AddRow(false))
	shared, err = SharesGroupContext(context.Background(), "user-1", "user-3")
	if err != nil || shared {
		t.Fatalf("disjoint groups = %v, %v", shared, err)
	}
}

func TestGroupListsMembersAndLeaderboard(t *testing.T) {
	mock := newMockPool(t)
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT g.id, g.name, g.code").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow("g1", "One", "AAA111", now))
	groups, err := GetUserGroupsContext(context.Background(), "user-1")
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups = %+v, %v", groups, err)
	}
	mock.ExpectQuery("SELECT g.id, g.name, g.code").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow("g1", "One", "AAA111", now))
	if groups, err := GetUserGroups("user-1"); err != nil || len(groups) != 1 {
		t.Fatalf("GetUserGroups = %+v, %v", groups, err)
	}
	mock.ExpectQuery("SELECT u.id, u.username, u.avatar").WithArgs("g1").WillReturnRows(pgxmock.NewRows([]string{"id", "username", "avatar"}).AddRow("u1", "alice", "a.png"))
	members, err := GetGroupMembers("g1")
	if err != nil || len(members) != 1 || members[0]["username"] != "alice" {
		t.Fatalf("members = %+v, %v", members, err)
	}
	leaderboardQuery := `(?s)SELECT u\.id, u\.username, u\.avatar.*SUM\(g\.score\).*ORDER BY COALESCE\(SUM\(g\.score\), 0\) DESC`
	mock.ExpectQuery(leaderboardQuery).WithArgs("g1").WillReturnRows(pgxmock.NewRows([]string{"id", "username", "avatar", "score", "count", "average", "total_points"}).AddRow("u1", "alice", "avatar4.png", 160, 2, 80.0, 7600))
	entries, err := GetGroupLeaderboardContext(context.Background(), "g1")
	if err != nil || len(entries) != 1 || entries[0].Score != 160 || entries[0].Average != 80.0 || entries[0].Avatar != "avatar4.png" || entries[0].TotalPoints != 7600 || entries[0].Rank.Name != "Knight" {
		t.Fatalf("leaderboard = %+v, %v", entries, err)
	}
	mock.ExpectQuery(leaderboardQuery).WithArgs("g1").WillReturnRows(pgxmock.NewRows([]string{"id", "username", "avatar", "score", "count", "average", "total_points"}).AddRow("u1", "alice", "avatar4.png", 160, 2, 80.0, 7600))
	if entries, err := GetGroupLeaderboard("g1"); err != nil || len(entries) != 1 {
		t.Fatalf("GetGroupLeaderboard = %+v, %v", entries, err)
	}
}

func TestLeaderboardPeriodStart(t *testing.T) {
	now := time.Date(2026, time.July, 26, 18, 45, 0, 0, time.FixedZone("CEST", 2*60*60))
	week := leaderboardPeriodStart(LeaderboardWeek, now)
	month := leaderboardPeriodStart(LeaderboardMonth, now)
	if week == nil || !week.Equal(time.Date(2026, time.July, 20, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("week start = %v", week)
	}
	if month == nil || !month.Equal(time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("month start = %v", month)
	}
	if start := leaderboardPeriodStart(LeaderboardAllTime, now); start != nil {
		t.Fatalf("all-time start = %v", start)
	}
}
