// Package database provides PostgreSQL connection management for the Card Issuer API.
//
// Security considerations:
//   - Connection strings are loaded from environment variables, never hardcoded.
//   - Each request sets the RLS session variable `app.current_tenant` before executing queries.
//   - Connection health is monitored via PingContext.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// DB wraps *sql.DB with application-specific helpers.
type DB struct {
	*sql.DB
	logger *slog.Logger
}

// New creates a new database connection pool with the given configuration.
func New(dsn string, maxOpen, maxIdle int, connMaxLifetime time.Duration, logger *slog.Logger) (*DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("database DSN is required")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(connMaxLifetime)

	// Verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	logger.Info("database connection established",
		"max_open_conns", maxOpen,
		"max_idle_conns", maxIdle,
		"conn_max_lifetime", connMaxLifetime.String(),
	)

	return &DB{DB: db, logger: logger}, nil
}

// SetTenantContext sets the PostgreSQL session variable for Row Level Security.
// This is called at the beginning of each request to ensure RLS policies
// filter rows by the current tenant.
//
// IMPORTANT: This is a defense-in-depth measure. The primary isolation mechanism
// is parameterized WHERE tenant_id = $1 clauses in every query. RLS is the
// last line of defense in case of application bugs.
func SetTenantContext(ctx context.Context, exec interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}, tenantID string) error {
	// We use a parameterized SET via format_type to prevent injection.
	// PostgreSQL's SET does not support parameterized queries directly,
	// so we use a DO block with format() for safe interpolation.
	_, err := exec.ExecContext(ctx,
		"SELECT set_config('app.current_tenant', $1, true)", // true = local to transaction
		tenantID,
	)
	if err != nil {
		return fmt.Errorf("failed to set tenant context: %w", err)
	}
	return nil
}

// HealthCheck verifies the database connection is alive.
func (db *DB) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

// Check satisfies the handler.HealthChecker interface.
func (db *DB) Check(ctx context.Context) error {
	return db.HealthCheck(ctx)
}

// Close gracefully shuts down the database connection pool.
func (db *DB) Close() error {
	db.logger.Info("closing database connection pool")
	return db.DB.Close()
}
