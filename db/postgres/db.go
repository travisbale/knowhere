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

// DB is a generic database wrapper that works with any sqlc-generated Queries type.
// Q is the queries type (e.g., *sqlc.Queries)
type DB[Q any] struct {
	pool *pgxpool.Pool
	newQ func(any) Q
}

// Config holds optional configuration for the database connection pool
type Config struct {
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	AfterConnect      func(ctx context.Context, conn *pgx.Conn) error
}

// DefaultConfig returns sensible defaults for connection pooling
func DefaultConfig() *Config {
	return &Config{
		MaxConns:          25,
		MinConns:          5,
		MaxConnLifetime:   time.Hour,
		MaxConnIdleTime:   30 * time.Minute,
		HealthCheckPeriod: time.Minute,
	}
}

// NewDB creates a new database connection pool with the given queries constructor.
// The newQ function should wrap sqlc.New, e.g.: func(d any) *sqlc.Queries { return sqlc.New(d.(sqlc.DBTX)) }
//
// Constructing a pool does not wait for the database. ctx governs the pool's background
// connection warm-up, so give it one that lives as long as the pool, not a startup timeout.
// A caller that needs the database to be reachable asks Health, as a readiness probe does.
func NewDB[Q any](ctx context.Context, databaseURL string, newQ func(any) Q, cfg *Config) (*DB[Q], error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing database URL %s: %w", databaseURL, err)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = cfg.HealthCheckPeriod
	poolConfig.AfterConnect = cfg.AfterConnect

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("creating database connection: %w", err)
	}

	return &DB[Q]{pool: pool, newQ: newQ}, nil
}

// Close closes the database connection pool
func (d *DB[Q]) Close() {
	d.pool.Close()
}

// Health checks if the database is healthy
func (d *DB[Q]) Health(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

// Queries returns a new Queries instance for executing SQL queries outside a transaction
func (d *DB[Q]) Queries() Q {
	return d.newQ(d.pool)
}

// Pool returns the underlying pgx connection pool
func (d *DB[Q]) Pool() *pgxpool.Pool {
	return d.pool
}

// WithTransaction executes a function within a database transaction.
// Use for operations that don't require tenant scoping.
func (d *DB[Q]) WithTransaction(ctx context.Context, fn func(Q) error) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			slog.Error("failed to rollback transaction", "error", err)
		}
	}()

	if err := fn(d.newQ(tx)); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// WithTenantContext executes a function within a tenant-scoped transaction.
// Sets app.current_tenant_id for Row Level Security policies.
func (d *DB[Q]) WithTenantContext(ctx context.Context, fn func(Q) error) error {
	tenantID, err := identity.GetTenant(ctx)
	if err != nil {
		return err
	}

	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && err != pgx.ErrTxClosed {
			slog.Error("failed to rollback transaction", "error", err)
		}
	}()

	// SET LOCAL ensures the tenant context lasts only for this transaction
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.current_tenant_id = '%s'", tenantID)); err != nil {
		return fmt.Errorf("failed to set tenant context: %w", err)
	}

	if err := fn(d.newQ(tx)); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
