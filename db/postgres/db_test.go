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

// These exercise real commit and rollback semantics, so they need a database. Set
// KNOWHERE_TEST_DATABASE_URL to run them; without it they skip rather than fail, because
// this repo carries no database of its own and `go test ./...` should stay useful.
const dbURLEnv = "KNOWHERE_TEST_DATABASE_URL"

// execer is the slice of pgx that both a pool and a transaction satisfy, which is the
// premise of the generic wrapper: the queries type cannot tell them apart.
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
		t.Skipf("set %s to run the postgres tests", dbURLEnv)
	}
	db, err := NewDB(context.Background(), url, newTestQueries, nil)
	if err != nil {
		t.Skipf("cannot reach %s (%v)", dbURLEnv, err)
	}
	t.Cleanup(db.Close)

	// A table per test keeps them parallel-safe and needs no migration to exist.
	table := "knowhere_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := db.Pool().Exec(context.Background(), fmt.Sprintf("CREATE TABLE %s (id uuid primary key)", table)); err != nil {
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

// The property every caller depends on: one closure is one transaction, so several
// statements inside it either all land or none do. It is what lets a repository method
// write to more than one table without leaving the second one behind.
func TestWithTenantContext_IsOneTransaction(t *testing.T) {
	db, table := testDB(t)
	ctx := identity.WithTenant(context.Background(), uuid.New())
	boom := errors.New("failed on the third statement")

	err := db.WithTenantContext(ctx, func(q *testQueries) error {
		if err := insert(ctx, q, table); err != nil {
			return err
		}
		if err := insert(ctx, q, table); err != nil {
			return err
		}
		return boom
	})

	if !errors.Is(err, boom) {
		t.Fatalf("want the caller's error surfaced, got %v", err)
	}
	if n := rowCount(t, db, table); n != 0 {
		t.Errorf("want both statements rolled back, %d row(s) survived", n)
	}
}

func TestWithTenantContext_CommitsWhenFnSucceeds(t *testing.T) {
	db, table := testDB(t)
	ctx := identity.WithTenant(context.Background(), uuid.New())

	if err := db.WithTenantContext(ctx, func(q *testQueries) error {
		if err := insert(ctx, q, table); err != nil {
			return err
		}
		return insert(ctx, q, table)
	}); err != nil {
		t.Fatalf("WithTenantContext: %v", err)
	}
	if n := rowCount(t, db, table); n != 2 {
		t.Errorf("want both writes committed, got %d", n)
	}
}

// Writes are invisible outside the closure until it returns, which is what makes a
// half-finished multi-step write unobservable rather than merely short-lived.
func TestWithTenantContext_WritesAreNotVisibleUntilCommit(t *testing.T) {
	db, table := testDB(t)
	ctx := identity.WithTenant(context.Background(), uuid.New())

	err := db.WithTenantContext(ctx, func(q *testQueries) error {
		if err := insert(ctx, q, table); err != nil {
			return err
		}
		if n := rowCount(t, db, table); n != 0 {
			t.Errorf("uncommitted write is visible on another connection (%d rows)", n)
		}
		return errors.New("roll it back")
	})
	if err == nil {
		t.Fatal("want the rollback error")
	}
}

// RLS is the isolation mechanism, so running without a tenant must fail rather than
// quietly execute against whatever scope the connection happens to have.
func TestWithTenantContext_RequiresATenant(t *testing.T) {
	db, table := testDB(t)

	err := db.WithTenantContext(context.Background(), func(q *testQueries) error {
		t.Error("the callback should not have run without a tenant")
		return insert(context.Background(), q, table)
	})
	if err == nil {
		t.Fatal("want an error when no tenant is in the context")
	}
	if n := rowCount(t, db, table); n != 0 {
		t.Errorf("nothing should have been written, got %d row(s)", n)
	}
}

// SET LOCAL is what scopes RLS, and LOCAL means "for this transaction" — so it has to be
// set inside the same transaction as the statements it governs, not on a pooled
// connection that a later caller might inherit.
func TestWithTenantContext_SetsTheTenantForTheTransaction(t *testing.T) {
	db, _ := testDB(t)
	tenantID := uuid.New()
	ctx := identity.WithTenant(context.Background(), tenantID)

	var seen string
	if err := db.WithTenantContext(ctx, func(q *testQueries) error {
		return q.conn.QueryRow(ctx, "SELECT current_setting('app.current_tenant_id', true)").Scan(&seen)
	}); err != nil {
		t.Fatalf("WithTenantContext: %v", err)
	}
	if seen != tenantID.String() {
		t.Errorf("want app.current_tenant_id = %s inside the transaction, got %q", tenantID, seen)
	}

	// And gone afterwards: a pooled connection must not carry one caller's tenant into
	// the next caller's work.
	var after string
	if err := db.Pool().QueryRow(context.Background(), "SELECT current_setting('app.current_tenant_id', true)").Scan(&after); err != nil {
		t.Fatalf("read setting after commit: %v", err)
	}
	if after == tenantID.String() {
		t.Error("the tenant setting outlived its transaction")
	}
}

// The untenanted sibling, used for anything not behind RLS.
func TestWithTransaction_RollsBackWithoutNeedingATenant(t *testing.T) {
	db, table := testDB(t)
	boom := errors.New("failed partway")

	err := db.WithTransaction(context.Background(), func(q *testQueries) error {
		if err := insert(context.Background(), q, table); err != nil {
			return err
		}
		return boom
	})

	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	if n := rowCount(t, db, table); n != 0 {
		t.Errorf("want the write rolled back, %d row(s) survived", n)
	}
}

// Queries() is the non-transactional path, and callers rely on that: a write through it
// stands on its own rather than waiting on a closure that never comes.
func TestQueries_WritesOutsideAnyTransaction(t *testing.T) {
	db, table := testDB(t)
	if err := insert(context.Background(), db.Queries(), table); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if n := rowCount(t, db, table); n != 1 {
		t.Errorf("want the write visible immediately, got %d", n)
	}
}
