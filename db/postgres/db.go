package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/travisbale/knowhere/identity"
)

// DB is a generic database wrapper that works with any sqlc-generated Queries type.
// Q is the queries type (e.g., *sqlc.Queries)
type DB[Q any] struct {
	pool *pgxpool.Pool
	newQ func(any) Q

	// tx is set only on the handle WithinTenantTx hands to its callback. Every method on
	// that handle runs on this transaction instead of taking a fresh connection from the
	// pool, which is what lets a multi-step operation commit or roll back as one.
	//
	// A pgx.Tx wraps a single connection and is not safe for concurrent use, so a bound
	// handle must not be shared across goroutines.
	tx pgx.Tx
	// tenantID is the tenant tx was opened for, kept so a nested call under a different
	// tenant is refused rather than silently running against this one's RLS scope.
	tenantID uuid.UUID
}

// conn returns the transaction when this handle is bound to one, and the pool otherwise.
func (d *DB[Q]) conn() any {
	if d.tx != nil {
		return d.tx
	}
	return d.pool
}

// bound reports whether this handle is already running inside a transaction.
func (d *DB[Q]) bound() bool { return d.tx != nil }

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

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return &DB[Q]{pool: pool, newQ: newQ}, nil
}

// Close closes the database connection pool. It is a no-op on a handle bound by
// WithinTenantTx, which borrows the pool rather than owning it — closing there would take
// the pool out from under every other caller.
func (d *DB[Q]) Close() {
	if d.bound() {
		return
	}
	d.pool.Close()
}

// Health checks if the database is healthy
func (d *DB[Q]) Health(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

// Queries returns a Queries instance for executing SQL. On a handle bound by
// WithinTenantTx it runs on that transaction; otherwise it goes straight to the pool and
// is not transactional.
func (d *DB[Q]) Queries() Q {
	return d.newQ(d.conn())
}

// Pool returns the underlying pgx connection pool
func (d *DB[Q]) Pool() *pgxpool.Pool {
	return d.pool
}

// WithTransaction executes a function within a database transaction.
// Use for operations that don't require tenant scoping.
func (d *DB[Q]) WithTransaction(ctx context.Context, fn func(Q) error) error {
	// Already inside one: run on it. Opening a second transaction on another connection
	// would not see this one's uncommitted writes, and committing it separately would
	// defeat the atomicity the caller asked for.
	if d.bound() {
		return fn(d.newQ(d.tx))
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

	if d.bound() {
		// SET LOCAL applies for the whole transaction, so a call arriving under a
		// different tenant would run against the enclosing tenant's RLS scope while
		// believing it was its own — rows silently missing rather than an error. Refuse.
		if tenantID != d.tenantID {
			return fmt.Errorf("tenant %s cannot be used inside a transaction opened for tenant %s", tenantID, d.tenantID)
		}
		return fn(d.newQ(d.tx))
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

// WithinTenantTx runs fn with a DB bound to a single tenant-scoped transaction. Every
// call made through the handle fn receives — including through repositories and services
// constructed over it — commits or rolls back together.
//
// This exists for operations that are one unit of work spread over many statements, where
// failing partway leaves state that is wrong rather than merely incomplete. A request
// handler wants the opposite and should keep using WithTenantContext: one edit, one
// transaction, released as soon as it is done.
//
// The transaction is a value rather than something hidden in the context, so a caller can
// see which handle is transactional and pass it deliberately. The handle wraps a single
// connection and must not be used from more than one goroutine.
//
// Nesting joins the outer transaction rather than failing: rolling back only an inner
// part would leave exactly the half-applied state this is meant to prevent.
func (d *DB[Q]) WithinTenantTx(ctx context.Context, fn func(*DB[Q]) error) error {
	if d.bound() {
		return fn(d)
	}

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

	// SET LOCAL ensures the tenant context lasts only for this transaction. tenantID is a
	// uuid.UUID, so its formatting cannot carry anything but hex and dashes — SET LOCAL
	// takes no bind parameters, which is why this is interpolated at all.
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL app.current_tenant_id = '%s'", tenantID)); err != nil {
		return fmt.Errorf("failed to set tenant context: %w", err)
	}

	if err := fn(&DB[Q]{pool: d.pool, newQ: d.newQ, tx: tx, tenantID: tenantID}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
