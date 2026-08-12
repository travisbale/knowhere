package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/travisbale/knowhere/identity"
)

// These exercise real transaction semantics, so they need a database. Set
// KNOWHERE_TEST_DATABASE_URL to run them; without it they skip rather than fail, because
// this repo carries no database of its own.
const dbURLEnv = "KNOWHERE_TEST_DATABASE_URL"

// execer is the slice of pgx that both a pool and a transaction satisfy, which is the
// whole premise of the generic wrapper: the queries type cannot tell them apart.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type testQueries struct{ conn execer }

func newTestQueries(d any) *testQueries { return &testQueries{conn: d.(execer)} }

func testDB(t *testing.T) (*DB[*testQueries], string) {
	t.Helper()
	url := os.Getenv(dbURLEnv)
	if url == "" {
		t.Skipf("set %s to run the postgres transaction tests", dbURLEnv)
	}
	db, err := NewDB(context.Background(), url, newTestQueries, nil)
	if err != nil {
		t.Skipf("cannot reach %s (%v)", dbURLEnv, err)
	}
	t.Cleanup(db.Close)

	// A table per test keeps them parallel-safe and needs no migration to exist.
	table := "knowhere_tx_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ctx := context.Background()
	if _, err := db.Pool().Exec(ctx, fmt.Sprintf("CREATE TABLE %s (id uuid primary key)", table)); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Pool().Exec(context.Background(), "DROP TABLE IF EXISTS "+table) })
	return db, table
}

func rowCount(t *testing.T, db *DB[*testQueries], table string) int {
	t.Helper()
	var n int
	if err := db.Pool().QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func insert(ctx context.Context, q *testQueries, table string) error {
	_, err := q.conn.Exec(ctx, fmt.Sprintf("INSERT INTO %s (id) VALUES ($1)", table), uuid.New())
	return err
}

// The point of the whole change: many statements, one outcome.
func TestWithinTenantTx_RollsBackEveryStatement(t *testing.T) {
	db, table := testDB(t)
	ctx := identity.WithTenant(context.Background(), uuid.New())
	boom := errors.New("failed partway")

	err := db.WithinTenantTx(ctx, func(tx *DB[*testQueries]) error {
		for range 3 {
			if err := insert(ctx, tx.Queries(), table); err != nil {
				return err
			}
		}
		return boom
	})

	if !errors.Is(err, boom) {
		t.Fatalf("want the caller's error surfaced, got %v", err)
	}
	if n := rowCount(t, db, table); n != 0 {
		t.Errorf("want every statement rolled back, %d row(s) survived", n)
	}
}

func TestWithinTenantTx_CommitsWhenFnSucceeds(t *testing.T) {
	db, table := testDB(t)
	ctx := identity.WithTenant(context.Background(), uuid.New())

	if err := db.WithinTenantTx(ctx, func(tx *DB[*testQueries]) error {
		return insert(ctx, tx.Queries(), table)
	}); err != nil {
		t.Fatalf("WithinTenantTx: %v", err)
	}
	if n := rowCount(t, db, table); n != 1 {
		t.Errorf("want the write committed, got %d row(s)", n)
	}
}

// A repository calling WithTenantContext must join the enclosing transaction, not open a
// second one on another connection — which would neither see these writes nor roll back
// with them.
func TestWithTenantContext_JoinsTheEnclosingTransaction(t *testing.T) {
	db, table := testDB(t)
	ctx := identity.WithTenant(context.Background(), uuid.New())
	boom := errors.New("failed after the nested write")

	err := db.WithinTenantTx(ctx, func(tx *DB[*testQueries]) error {
		if err := insert(ctx, tx.Queries(), table); err != nil {
			return err
		}
		// Exactly what a repo method does.
		if err := tx.WithTenantContext(ctx, func(q *testQueries) error {
			var n int
			// Sees the outer write, so it is the same transaction.
			if err := q.conn.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				return fmt.Errorf("nested call sees %d row(s), want the enclosing write", n)
			}
			return insert(ctx, q, table)
		}); err != nil {
			return err
		}
		return boom
	})

	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if n := rowCount(t, db, table); n != 0 {
		t.Errorf("want the nested write rolled back too, %d row(s) survived", n)
	}
}

// SET LOCAL applies for the whole transaction, so a nested call under another tenant would
// run against the enclosing tenant's RLS scope while believing it was its own — rows
// quietly missing instead of an error. It has to be refused.
func TestWithTenantContext_RefusesADifferentTenantInsideATransaction(t *testing.T) {
	db, _ := testDB(t)
	outer, inner := uuid.New(), uuid.New()

	err := db.WithinTenantTx(identity.WithTenant(context.Background(), outer), func(tx *DB[*testQueries]) error {
		return tx.WithTenantContext(identity.WithTenant(context.Background(), inner), func(*testQueries) error {
			t.Error("the nested call should not have run")
			return nil
		})
	})

	if err == nil || !strings.Contains(err.Error(), outer.String()) || !strings.Contains(err.Error(), inner.String()) {
		t.Fatalf("want an error naming both tenants, got %v", err)
	}
}

// Queries() on a bound handle has to follow the transaction; going to the pool would let a
// caller escape it without any sign at the call site.
func TestQueries_FollowsTheTransaction(t *testing.T) {
	db, table := testDB(t)
	ctx := identity.WithTenant(context.Background(), uuid.New())

	err := db.WithinTenantTx(ctx, func(tx *DB[*testQueries]) error {
		if err := insert(ctx, tx.Queries(), table); err != nil {
			return err
		}
		// Uncommitted, so a pool connection cannot see it yet.
		if n := rowCount(t, db, table); n != 0 {
			t.Errorf("write is visible outside the transaction before commit (%d rows)", n)
		}
		return errors.New("roll it back")
	})
	if err == nil {
		t.Fatal("want the rollback error")
	}
}

// The bound handle borrows the pool rather than owning it, so closing it would take the
// pool out from under everything else.
func TestClose_IsANoOpOnABoundHandle(t *testing.T) {
	db, table := testDB(t)
	ctx := identity.WithTenant(context.Background(), uuid.New())

	if err := db.WithinTenantTx(ctx, func(tx *DB[*testQueries]) error {
		tx.Close()
		return nil
	}); err != nil {
		t.Fatalf("WithinTenantTx: %v", err)
	}
	// The pool is still usable afterwards.
	if n := rowCount(t, db, table); n != 0 {
		t.Errorf("unexpected rows: %d", n)
	}
}

// Nesting joins rather than failing: rolling back only an inner part would leave exactly
// the half-applied state this is meant to prevent.
func TestWithinTenantTx_NestsByJoining(t *testing.T) {
	db, table := testDB(t)
	ctx := identity.WithTenant(context.Background(), uuid.New())
	boom := errors.New("outer failure")

	err := db.WithinTenantTx(ctx, func(outer *DB[*testQueries]) error {
		if err := outer.WithinTenantTx(ctx, func(inner *DB[*testQueries]) error {
			return insert(ctx, inner.Queries(), table)
		}); err != nil {
			return err
		}
		return boom
	})

	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	// The inner call returned nil, but it must not have committed on its own.
	if n := rowCount(t, db, table); n != 0 {
		t.Errorf("inner transaction committed independently, %d row(s) survived", n)
	}
}
