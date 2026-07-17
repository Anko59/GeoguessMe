package database

import (
	"context"
	"log"
	"log/slog"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"
)

func TestMigrationDiscoveryAndDisconnectedDatabase(t *testing.T) {
	all, err := migrations()
	if err != nil || len(all) != 3 || all[0].Version != 1 || all[1].Version != 2 || all[2].Version != 3 {
		t.Fatalf("migrations = %+v, %v", all, err)
	}
	if err := Connect(""); err == nil {
		t.Fatal("empty database URL accepted")
	}
	if err := ConnectWithLimits("://invalid", 0, 1); err == nil {
		t.Fatal("invalid database URL accepted")
	}
	DB = nil
	if err := MigrateUp(context.Background(), nil); err == nil || err.Error() != "database is not connected" {
		t.Fatalf("disconnected migration error = %v", err)
	}
	if _, err := MigrationStatus(context.Background()); err == nil {
		t.Fatal("disconnected status unexpectedly succeeded")
	}
	Close()
	InitSchema()
	log.Print("database compatibility helpers exercised")
}

func TestMigrationStatusUsesPool(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	previous := DB
	DB = mock
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
		DB = previous
	})
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	when := time.Now().UTC()
	mock.ExpectQuery("SELECT version, applied_at FROM schema_migrations").WillReturnRows(pgxmock.NewRows([]string{"version", "applied_at"}).AddRow(1, when))
	records, err := MigrationStatus(context.Background())
	if err != nil || len(records) != 3 || !records[0].Applied || records[1].Applied || records[2].Applied {
		t.Fatalf("migration records = %+v, %v", records, err)
	}
}

func TestMigrateUpSkipsAppliedMigrations(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	previous := DB
	previousAcquire := acquireMigrationConnection
	DB = mock
	acquireMigrationConnection = func(context.Context) (migrationConnection, error) {
		return migrationMockConnection{PgxPoolIface: mock}, nil
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
		DB = previous
		acquireMigrationConnection = previousAcquire
	})
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS schema_migrations").WillReturnResult(pgxmock.NewResult("CREATE", 0))
	mock.ExpectExec("SELECT pg_advisory_lock\\(\\$1\\)").WithArgs(migrationLockKey).WillReturnResult(pgxmock.NewResult("SELECT", 1))
	mock.ExpectQuery("SELECT version FROM schema_migrations").WillReturnRows(
		pgxmock.NewRows([]string{"version"}).AddRow(1).AddRow(2).AddRow(3),
	)
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").WithArgs(migrationLockKey).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	if err := MigrateUp(context.Background(), slog.Default()); err != nil {
		t.Fatalf("MigrateUp returned an error: %v", err)
	}
}

func TestMigrateUpAppliesPendingMigrations(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	previous := DB
	previousAcquire := acquireMigrationConnection
	DB = mock
	acquireMigrationConnection = func(context.Context) (migrationConnection, error) {
		return migrationMockConnection{PgxPoolIface: mock}, nil
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		mock.Close()
		DB = previous
		acquireMigrationConnection = previousAcquire
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
	mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").WithArgs(migrationLockKey).WillReturnResult(pgxmock.NewResult("SELECT", 1))

	if err := MigrateUp(context.Background(), slog.Default()); err != nil {
		t.Fatalf("MigrateUp returned an error: %v", err)
	}
}

type migrationMockConnection struct {
	pgxmock.PgxPoolIface
}

func (migrationMockConnection) Release() {}
