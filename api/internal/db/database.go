// Package db owns PostgreSQL connectivity and schema migrations.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationFS keeps migrations inside the API binary so the container does
// not depend on the host filesystem after it has been built.
//
//go:embed migrations/*.sql
var migrationFS embed.FS

const migrationTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version BIGINT PRIMARY KEY,
    name TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`

const migrationLockKey int64 = 0x48595045524e4f56

// Open creates a bounded PostgreSQL connection pool. The caller owns the pool
// and must call Close when the process is shutting down.
func Open(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	config.MaxConns = 10
	config.MinConns = 1
	config.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate applies embedded migrations in numeric filename order. Each file
// runs in its own transaction and is recorded only after it commits.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return fmt.Errorf("database pool is required")
	}
	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration lock connection: %w", err)
	}
	defer lockConn.Release()
	if _, err := lockConn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() { _, _ = lockConn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey) }()

	if _, err := lockConn.Exec(ctx, migrationTable); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	files, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Slice(files, func(i, j int) bool { return migrationVersion(files[i]) < migrationVersion(files[j]) })

	for _, file := range files {
		version, err := parseMigrationVersion(file)
		if err != nil {
			return err
		}

		var applied bool
		if err := lockConn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", file, err)
		}
		if applied {
			continue
		}

		contents, err := migrationFS.ReadFile(file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}
		tx, err := lockConn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", file, err)
		}
		if _, err := tx.Exec(ctx, string(contents)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", file, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version, name) VALUES ($1, $2)", version, filepath.Base(file)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", file, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", file, err)
		}
	}
	return nil
}

func migrationVersion(file string) int64 {
	version, _ := parseMigrationVersion(file)
	return version
}

func parseMigrationVersion(file string) (int64, error) {
	name := filepath.Base(file)
	prefix := strings.SplitN(name, "_", 2)[0]
	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid migration filename %q: %w", name, err)
	}
	return version, nil
}
