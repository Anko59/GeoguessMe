package database

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log"
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

// DB is retained as the repository seam while the application is migrated to
// dependency-injected services. New code should pass request contexts through
// repository calls.
type Pool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Begin(context.Context) (pgx.Tx, error)
	Acquire(context.Context) (*pgxpool.Conn, error)
	Ping(context.Context) error
	Close()
}

type migrationConnection interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Begin(context.Context) (pgx.Tx, error)
	Release()
}

var DB Pool

var acquireMigrationConnection = func(ctx context.Context) (migrationConnection, error) {
	if DB == nil {
		return nil, errors.New("database is not connected")
	}
	return DB.Acquire(ctx)
}

var _ Pool = (*pgxpool.Pool)(nil)

func Connect(dbURL string) error {
	return ConnectWithLimits(dbURL, 2, 10)
}

func ConnectWithLimits(dbURL string, minConns, maxConns int32) error {
	if dbURL == "" {
		return errors.New("DATABASE_URL is not set")
	}
	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		return fmt.Errorf("parse database URL: %w", err)
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
		return fmt.Errorf("connect to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("ping database: %w", err)
	}
	DB = pool
	return nil
}

func Close() {
	if DB != nil {
		DB.Close()
		DB = nil
	}
}

//go:embed migrations/*.sql
var migrationFS embed.FS

type Migration struct {
	Version int
	Name    string
	SQL     string
}

func migrations() ([]Migration, error) {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return nil, err
	}
	var result []Migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		var version int
		var name string
		if _, err := fmt.Sscanf(entry.Name(), "%d_%s", &version, &name); err != nil {
			return nil, fmt.Errorf("invalid migration filename %q: %w", entry.Name(), err)
		}
		contents, err := fs.ReadFile(migrationFS, "migrations/"+entry.Name())
		if err != nil {
			return nil, err
		}
		result = append(result, Migration{Version: version, Name: strings.TrimSuffix(name, ".sql"), SQL: string(contents)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	return result, nil
}

func ensureMigrationTable(ctx context.Context) error {
	_, err := DB.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	return err
}

func MigrateUp(ctx context.Context, logger *slog.Logger) error {
	if DB == nil {
		return errors.New("database is not connected")
	}
	if err := ensureMigrationTable(ctx); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	conn, err := acquireMigrationConnection(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()
	// Hold a session-level advisory lock for the whole run so concurrent
	// processes wait rather than racing on schema_migrations.
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

func MigrationStatus(ctx context.Context) ([]MigrationRecord, error) {
	if DB == nil {
		return nil, errors.New("database is not connected")
	}
	if err := ensureMigrationTable(ctx); err != nil {
		return nil, err
	}
	appliedAt := make(map[int]time.Time)
	rows, err := DB.Query(ctx, `SELECT version, applied_at FROM schema_migrations`)
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

// InitSchema is deliberately kept only for source compatibility with older
// callers. Server startup must call MigrateUp explicitly.
func InitSchema() {
	logFatalDeprecated()
}

func logFatalDeprecated() {
	log.Printf("InitSchema is deprecated; run 'geoguessme migrate up'")
}
