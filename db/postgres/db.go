package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/travisbale/knowhere/identity"
)

// DB wraps the pgx connection pool with multi-tenant transaction support
type DB struct {
	pool *pgxpool.Pool
}

// Config holds database connection configuration
type Config struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	// AfterConnect is called after establishing a new connection (useful for loading custom types)
	AfterConnect func(ctx context.Context, conn *pgx.Conn) error
}

// DefaultConfig returns sensible default configuration values
func DefaultConfig() *Config {
	return &Config{
		MaxConns:          25,
		MinConns:          5,
		MaxConnLifetime:   time.Hour,
		MaxConnIdleTime:   30 * time.Minute,
		HealthCheckPeriod: time.Minute,
	}
}

// NewDB creates a new database connection pool with default configuration
func NewDB(ctx context.Context, databaseURL string) (*DB, error) {
	return NewDBWithConfig(ctx, databaseURL, DefaultConfig())
}

// NewDBWithConfig creates a new database connection pool with custom configuration
func NewDBWithConfig(ctx context.Context, databaseURL string, cfg *Config) (*DB, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	config.MaxConns = cfg.MaxConns
	config.MinConns = cfg.MinConns
	config.MaxConnLifetime = cfg.MaxConnLifetime
	config.MaxConnIdleTime = cfg.MaxConnIdleTime
	config.HealthCheckPeriod = cfg.HealthCheckPeriod

	if cfg.AfterConnect != nil {
		config.AfterConnect = cfg.AfterConnect
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{pool: pool}, nil
}

// Close closes the database connection pool
func (d *DB) Close() {
	d.pool.Close()
}

// Health checks if the database is healthy
func (d *DB) Health(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

// Pool returns the underlying pgx connection pool
func (d *DB) Pool() *pgxpool.Pool {
	return d.pool
}

// WithTransaction executes a function within a database transaction.
// Use for pre-authentication operations where tenant context is unavailable
func (d *DB) WithTransaction(ctx context.Context, fn func(pgx.Tx) error) error {
	// Empty string signals cross-tenant operation for RLS policies
	return d.withTenantTransaction(ctx, "", fn)
}

// WithTenantContext executes a function within a tenant-scoped transaction.
// Sets app.current_tenant_id for Row Level Security policies
func (d *DB) WithTenantContext(ctx context.Context, fn func(pgx.Tx) error) error {
	tenantID, err := identity.GetTenant(ctx)
	if err != nil {
		return err
	}

	return d.withTenantTransaction(ctx, tenantID.String(), fn)
}

// withTenantTransaction executes a function within a transaction with the specified tenant ID
func (d *DB) withTenantTransaction(ctx context.Context, tenantID string, fn func(pgx.Tx) error) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	// Deferred rollback is safe even if transaction commits successfully
	defer func() {
		if err2 := tx.Rollback(ctx); err2 != nil && err2 != pgx.ErrTxClosed {
			slog.Error("failed to rollback transaction", "error", err2)
		}
	}()

	// SET LOCAL ensures the tenant context lasts only for this transaction
	query := fmt.Sprintf("SET LOCAL app.current_tenant_id = '%s'", tenantID)
	if _, err = tx.Exec(ctx, query); err != nil {
		return fmt.Errorf("failed to set tenant context: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
