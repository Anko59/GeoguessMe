package repository

import (
	"context"
	"testing"
	"time"

	"geoguessme/internal/models"

	"github.com/pashagolub/pgxmock/v4"
)

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

func TestGroupListsMembersAndLeaderboard(t *testing.T) {
	mock := newMockPool(t)
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT g.id, g.name, g.code").WithArgs("user-1").WillReturnRows(pgxmock.NewRows([]string{"id", "name", "code", "created_at"}).AddRow("g1", "One", "AAA111", now))
	groups, err := GetUserGroupsContext(context.Background(), "user-1")
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups = %+v, %v", groups, err)
	}
	mock.ExpectQuery("SELECT u.id, u.username, u.avatar").WithArgs("g1").WillReturnRows(pgxmock.NewRows([]string{"id", "username", "avatar"}).AddRow("u1", "alice", "a.png"))
	members, err := GetGroupMembers("g1")
	if err != nil || len(members) != 1 || members[0]["username"] != "alice" {
		t.Fatalf("members = %+v, %v", members, err)
	}
	mock.ExpectQuery("SELECT u.id, u.username").WithArgs("g1").WillReturnRows(pgxmock.NewRows([]string{"id", "username", "score", "count", "average"}).AddRow("u1", "alice", 80, 2, 80.5))
	entries, err := GetGroupLeaderboardContext(context.Background(), "g1")
	if err != nil || len(entries) != 1 || entries[0].Score != 80 {
		t.Fatalf("leaderboard = %+v, %v", entries, err)
	}
}
