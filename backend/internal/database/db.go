package database

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationLockKey is a stable PostgreSQL advisory-lock key so only one process
// applies migrations at a time even across replicas.
const migrationLockKey = 91734721

// Pool is the minimal PostgreSQL connection pool interface consumed by the
// repository slices. The concrete *pgxpool.Pool satisfies it.
type Pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Begin(context.Context) (pgx.Tx, error)
	Acquire(context.Context) (*pgxpool.Conn, error)
	Ping(context.Context) error
	Close()
}

var _ Pool = (*pgxpool.Pool)(nil)

// Connect opens a pool with the default connection limits.
func Connect(dbURL string) (Pool, error) {
	return ConnectWithLimits(dbURL, 2, 10)
}

// ConnectWithLimits opens a pool bounded to the given connection limits and
// verifies it with a ping. The pool is returned to the caller; the database
// package keeps no global pool reference.
func ConnectWithLimits(dbURL string, minConns, maxConns int32) (Pool, error) {
	if dbURL == "" {
		return nil, errors.New("DATABASE_URL is not set")
	}
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	if minConns >= 0 {
		cfg.MinConns = minConns
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

//go:embed migrations
var migrationFS embed.FS

type Migration struct {
	Version int
	Name    string
	SQL     string
}

// migrations discovers every committed migration under migrations/, including
// nested year subdirectories, so the directory stays within the repository's
// 14-direct-children structure limit while remaining forward-only.  Version
// and name are derived from the file name alone, so relocating a file never
// changes the schema_migrations identity of an applied migration.
func migrations() ([]Migration, error) {
	var result []Migration
	err := fs.WalkDir(migrationFS, "migrations", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".sql") {
			return nil
		}
		var version int
		var name string
		if _, err := fmt.Sscanf(d.Name(), "%d_%s", &version, &name); err != nil {
			return fmt.Errorf("invalid migration filename %q: %w", d.Name(), err)
		}
		contents, err := fs.ReadFile(migrationFS, path)
		if err != nil {
			return err
		}
		result = append(result, Migration{Version: version, Name: strings.TrimSuffix(name, ".sql"), SQL: string(contents)})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	return result, nil
}

type migrationConnection interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Begin(context.Context) (pgx.Tx, error)
}

func ensureMigrationTable(ctx context.Context, conn migrationConnection) error {
	_, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	return err
}

// MigrateUp applies pending forward-only migrations against the given pool.
// It takes a session-level advisory lock for the whole run so concurrent
// processes wait rather than racing on schema_migrations; the lock is released
// when the run finishes (including on failure).
func MigrateUp(ctx context.Context, pool Pool, logger *slog.Logger) error {
	if pool == nil {
		return errors.New("database is not connected")
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()
	return migrateUpOnConnection(ctx, conn, logger)
}

// migrateUpOnConnection keeps the session advisory lock, migration queries,
// and transactions on one physical PostgreSQL connection. Session locks do
// not protect work issued through arbitrary connections from the same pool.
func migrateUpOnConnection(ctx context.Context, conn migrationConnection, logger *slog.Logger) error {
	if err := ensureMigrationTable(ctx, conn); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
		return fmt.Errorf("acquire migration advisory lock: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockKey) }()

	all, err := migrations()
	if err != nil {
		return err
	}
	applied := make(map[int]bool, len(all))
	rows, err := conn.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			rows.Close()
			return err
		}
		applied[version] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, migration := range all {
		if applied[migration.Version] {
			continue
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %03d_%s: %w", migration.Version, migration.Name, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version, name) VALUES ($1, $2)`, migration.Version, migration.Name); err != nil {
			_ = tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		if logger != nil {
			logger.Info("database migration applied", "version", migration.Version, "name", migration.Name)
		}
	}
	return nil
}

func MigrationStatus(ctx context.Context, pool Pool) ([]MigrationRecord, error) {
	if pool == nil {
		return nil, errors.New("database is not connected")
	}
	if err := ensureMigrationTable(ctx, pool); err != nil {
		return nil, err
	}
	appliedAt := make(map[int]time.Time)
	rows, err := pool.Query(ctx, `SELECT version, applied_at FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var version int
		var at time.Time
		if err := rows.Scan(&version, &at); err != nil {
			rows.Close()
			return nil, err
		}
		appliedAt[version] = at
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	all, err := migrations()
	if err != nil {
		return nil, err
	}
	result := make([]MigrationRecord, 0, len(all))
	for _, migration := range all {
		record := MigrationRecord{Version: migration.Version, Name: migration.Name, Applied: false}
		if at, ok := appliedAt[migration.Version]; ok {
			record.Applied = true
			record.AppliedAt = at
		}
		result = append(result, record)
	}
	return result, nil
}

type MigrationRecord struct {
	Version   int
	Name      string
	Applied   bool
	AppliedAt time.Time
}
