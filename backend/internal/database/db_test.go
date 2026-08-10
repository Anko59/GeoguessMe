package database

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
)

func TestMigrationDiscoveryAndDisconnectedDatabase(t *testing.T) {
	all, err := migrations()
	if err != nil || len(all) != 16 || all[0].Version != 1 || all[1].Version != 2 || all[2].Version != 3 || all[3].Version != 4 || all[4].Version != 5 || all[5].Version != 6 || all[6].Version != 7 || all[7].Version != 8 || all[8].Version != 9 || all[9].Version != 10 || all[10].Version != 11 || all[11].Version != 12 || all[12].Version != 13 || all[13].Version != 14 || all[14].Version != 15 || all[15].Version != 16 {
		t.Fatalf("migrations = %+v, %v", all, err)
	}
	if _, err := Connect(""); err == nil {
		t.Fatal("empty database URL accepted")
	}
	if _, err := ConnectWithLimits("://invalid", 0, 1); err == nil {
		t.Fatal("invalid database URL accepted")
	}
	if err := MigrateUp(context.Background(), nil, nil); err == nil || err.Error() != "database is not connected" {
		t.Fatalf("disconnected migration error = %v", err)
	}
	if _, err := MigrationStatus(context.Background(), nil); err == nil {
		t.Fatal("disconnected status unexpectedly succeeded")
	}
}

func TestMigrationStatusUsesPool(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
	})
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	when := time.Now().UTC()
	mock.ExpectQuery("SELECT version, applied_at FROM schema_migrations").WillReturnRows(pgxmock.NewRows([]string{"version", "applied_at"}).AddRow(1, when))
	records, err := MigrationStatus(context.Background(), mock)
	if err != nil || len(records) != 15 || !records[0].Applied || records[1].Applied || records[2].Applied || records[3].Applied || records[4].Applied || records[5].Applied || records[6].Applied || records[7].Applied || records[8].Applied || records[9].Applied || records[10].Applied || records[11].Applied || records[12].Applied || records[13].Applied || records[14].Applied {
		t.Fatalf("migration records = %+v, %v", records, err)
	}
}

func TestMigrateUpSkipsAppliedMigrations(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
	})
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectExec("SELECT pg_advisory_lock\\(\\$1\\)").WithArgs(migrationLockKey).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("SELECT version FROM schema_migrations").WillReturnRows(
		pgxmock.NewRows([]string{"version"}).AddRow(1).AddRow(2).AddRow(3).AddRow(4).AddRow(5).AddRow(6).AddRow(7).AddRow(8).AddRow(9).AddRow(10).AddRow(11).AddRow(12).AddRow(13).AddRow(14).AddRow(15).AddRow(16),
	)
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").WithArgs(migrationLockKey).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	if err := migrateUpOnConnection(context.Background(), mock.AsConn(), slog.Default()); err != nil {
		t.Fatalf("migrateUpOnConnection returned an error: %v", err)
	}
}

func TestMigrateUpReportsConnectionAcquisitionFailure(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mock.Close)

	if err := MigrateUp(context.Background(), mock, slog.Default()); err == nil || !strings.Contains(err.Error(), "acquire migration connection") {
		t.Fatalf("expected connection acquisition failure, got %v", err)
	}
}

func TestMigrateUpAppliesPendingMigrations(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
	})
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectExec("SELECT pg_advisory_lock\\(\\$1\\)").WithArgs(migrationLockKey).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("SELECT version FROM schema_migrations").WillReturnRows(pgxmock.NewRows([]string{"version"}))
	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS users").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(1, "initial").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE users ADD COLUMN IF NOT EXISTS auth_version").WillReturnResult(pgxmock.NewResult("ALTER", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(2, "auth_version_and_object_deletion").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM media_deletion_jobs").WillReturnResult(pgxmock.NewResult("DELETE", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(3, "unique_active_media_deletion_job").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS push_subscriptions").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(4, "push_subscriptions").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS guesses_group_user_created_idx").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(5, "leaderboard_lookup_index").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("ADD COLUMN IF NOT EXISTS reply_to_id").WillReturnResult(pgxmock.NewResult("ALTER", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(6, "message_replies").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS chat_media").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(7, "chat_media").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS message_reactions").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(8, "message_reactions").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS group_photos").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(9, "group_settings").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS guesses_user_created_idx").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(10, "progression_lookup_index").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE message_reactions ADD COLUMN IF NOT EXISTS reaction").WillReturnResult(pgxmock.NewResult("ALTER", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(11, "custom_reactions").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE challenge_views ADD COLUMN IF NOT EXISTS media_delivered_at").WillReturnResult(pgxmock.NewResult("ALTER", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(12, "media_delivery_window").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE photos ADD COLUMN IF NOT EXISTS hide_location").WillReturnResult(pgxmock.NewResult("ALTER", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(13, "challenge_hide_location").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("DROP TRIGGER IF EXISTS sync_message_reaction_columns").WillReturnResult(pgxmock.NewResult("DROP", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(14, "retire_legacy_reaction_emoji").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE websocket_tickets ADD COLUMN IF NOT EXISTS auth_version(?s:.*DELETE FROM websocket_tickets)").WillReturnResult(pgxmock.NewResult("ALTER", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(15, "websocket_ticket_auth_version").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectBegin()
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS push_subscriptions_used_at_idx").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(16, "push_subscription_expiry_index").WillReturnResult(pgxmock.NewResult("INSERT", 1))
	mock.ExpectCommit()
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").WithArgs(migrationLockKey).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	if err := migrateUpOnConnection(context.Background(), mock.AsConn(), slog.Default()); err != nil {
		t.Fatalf("migrateUpOnConnection returned an error: %v", err)
	}
}
