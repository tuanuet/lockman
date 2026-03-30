package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"lockman/guard"
	pgguard "lockman/guard/postgres"
)

const integrationTimeout = 5 * time.Second

func TestIntegrationExistingRowUpdateAppliesNewerToken(t *testing.T) {
	ctx := testContext(t)
	db := openPostgresForTest(t)
	tableName := createOrdersTableForTest(t, db)

	insertOrderRow(t, db, tableName, "order-1", "order:123", "pending", 5, "worker-a")

	g := guard.Context{
		LockID:       "StrictOrderClaim",
		ResourceKey:  "order:123",
		FencingToken: 6,
		OwnerID:      "worker-b",
	}
	status := runGuardedExistingRowUpdate(t, ctx, db, tableName, g, "order-1", "paid")

	outcome, err := pgguard.ClassifyExistingRowUpdate(g, status)
	if err != nil {
		t.Fatalf("ClassifyExistingRowUpdate returned error: %v", err)
	}
	if outcome != guard.OutcomeApplied {
		t.Fatalf("expected %q, got %q", guard.OutcomeApplied, outcome)
	}
}

func TestIntegrationExistingRowUpdateRejectsOlderTokenAsStale(t *testing.T) {
	ctx := testContext(t)
	db := openPostgresForTest(t)
	tableName := createOrdersTableForTest(t, db)

	insertOrderRow(t, db, tableName, "order-1", "order:123", "pending", 5, "worker-a")

	g := guard.Context{
		LockID:       "StrictOrderClaim",
		ResourceKey:  "order:123",
		FencingToken: 4,
		OwnerID:      "worker-b",
	}
	status := runGuardedExistingRowUpdate(t, ctx, db, tableName, g, "order-1", "paid")

	outcome, err := pgguard.ClassifyExistingRowUpdate(g, status)
	if err != nil {
		t.Fatalf("ClassifyExistingRowUpdate returned error: %v", err)
	}
	if outcome != guard.OutcomeStaleRejected {
		t.Fatalf("expected %q, got %q", guard.OutcomeStaleRejected, outcome)
	}
}

func TestIntegrationExistingRowUpdateRejectsEqualTokenAsStale(t *testing.T) {
	ctx := testContext(t)
	db := openPostgresForTest(t)
	tableName := createOrdersTableForTest(t, db)

	insertOrderRow(t, db, tableName, "order-1", "order:123", "pending", 5, "worker-a")

	g := guard.Context{
		LockID:       "StrictOrderClaim",
		ResourceKey:  "order:123",
		FencingToken: 5,
		OwnerID:      "worker-b",
	}
	status := runGuardedExistingRowUpdate(t, ctx, db, tableName, g, "order-1", "paid")

	outcome, err := pgguard.ClassifyExistingRowUpdate(g, status)
	if err != nil {
		t.Fatalf("ClassifyExistingRowUpdate returned error: %v", err)
	}
	if outcome != guard.OutcomeStaleRejected {
		t.Fatalf("expected %q, got %q", guard.OutcomeStaleRejected, outcome)
	}
}

func TestIntegrationExistingRowUpdateRejectsMissingRowAsInvariant(t *testing.T) {
	ctx := testContext(t)
	db := openPostgresForTest(t)
	tableName := createOrdersTableForTest(t, db)

	g := guard.Context{
		LockID:       "StrictOrderClaim",
		ResourceKey:  "order:123",
		FencingToken: 1,
		OwnerID:      "worker-a",
	}
	status := runGuardedExistingRowUpdate(t, ctx, db, tableName, g, "missing-order", "paid")

	_, err := pgguard.ClassifyExistingRowUpdate(g, status)
	if !errors.Is(err, guard.ErrInvariantRejected) {
		t.Fatalf("expected invariant rejection, got %v", err)
	}
}

func TestIntegrationExistingRowUpdateRejectsBoundaryMismatchAsInvariant(t *testing.T) {
	ctx := testContext(t)
	db := openPostgresForTest(t)
	tableName := createOrdersTableForTest(t, db)

	insertOrderRow(t, db, tableName, "order-1", "order:123", "pending", 5, "worker-a")

	g := guard.Context{
		LockID:       "OtherStrictOrderClaim",
		ResourceKey:  "order:123",
		FencingToken: 6,
		OwnerID:      "worker-b",
	}
	status := runGuardedExistingRowUpdate(t, ctx, db, tableName, g, "order-1", "paid")

	_, err := pgguard.ClassifyExistingRowUpdate(g, status)
	if !errors.Is(err, guard.ErrInvariantRejected) {
		t.Fatalf("expected invariant rejection, got %v", err)
	}
}

func openPostgresForTest(t *testing.T) *sql.DB {
	t.Helper()

	dsn := strings.TrimSpace(os.Getenv("LOCKMAN_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("LOCKMAN_POSTGRES_DSN is not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open returned error: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := db.PingContext(testContext(t)); err != nil {
		t.Fatalf("PingContext returned error: %v", err)
	}

	return db
}

func createOrdersTableForTest(t *testing.T, db *sql.DB) string {
	t.Helper()

	tableName := fmt.Sprintf("orders_integration_%d", time.Now().UnixNano())
	createStmt := fmt.Sprintf(`
CREATE TABLE %s (
  id TEXT PRIMARY KEY,
  strict_lock_id TEXT NOT NULL,
  resource_key TEXT NOT NULL,
  status TEXT NOT NULL,
  last_fencing_token BIGINT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_by_owner TEXT NOT NULL
)`, tableName)
	if _, err := db.ExecContext(testContext(t), createStmt); err != nil {
		t.Fatalf("create table returned error: %v", err)
	}

	t.Cleanup(func() {
		dropStmt := fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)
		_, _ = db.ExecContext(testContext(t), dropStmt)
	})

	return tableName
}

func insertOrderRow(t *testing.T, db *sql.DB, tableName, orderID, resourceKey, status string, token uint64, owner string) {
	t.Helper()

	insertStmt := fmt.Sprintf(`
INSERT INTO %s (id, strict_lock_id, resource_key, status, last_fencing_token, updated_by_owner)
VALUES ($1, $2, $3, $4, $5, $6)
`, tableName)
	if _, err := db.ExecContext(
		testContext(t),
		insertStmt,
		orderID,
		"StrictOrderClaim",
		resourceKey,
		status,
		int64(token),
		owner,
	); err != nil {
		t.Fatalf("insert row returned error: %v", err)
	}
}

func runGuardedExistingRowUpdate(t *testing.T, ctx context.Context, db *sql.DB, tableName string, g guard.Context, orderID, status string) pgguard.ExistingRowStatus {
	t.Helper()

	query := fmt.Sprintf(`
WITH target AS (
  SELECT id, strict_lock_id, resource_key, last_fencing_token
  FROM %s
  WHERE id = $4
),
updated AS (
  UPDATE %s
  SET
    status = $1,
    last_fencing_token = $2,
    updated_at = NOW(),
    updated_by_owner = $3
  WHERE id = $4
    AND resource_key = $5
    AND strict_lock_id = $6
    AND last_fencing_token < $2
  RETURNING id
)
SELECT
  EXISTS(SELECT 1 FROM target) AS found,
  EXISTS(SELECT 1 FROM updated) AS applied,
  COALESCE((SELECT last_fencing_token FROM target), 0) AS current_token,
  COALESCE((SELECT resource_key FROM target), '') AS current_resource_key,
  COALESCE((SELECT strict_lock_id FROM target), '') AS current_lock_id
`, tableName, tableName)

	row := db.QueryRowContext(
		ctx,
		query,
		status,
		int64(g.FencingToken),
		g.OwnerID,
		orderID,
		g.ResourceKey,
		g.LockID,
	)
	got, err := pgguard.ScanExistingRowStatus(row)
	if err != nil {
		t.Fatalf("ScanExistingRowStatus returned error: %v", err)
	}
	return got
}

func testContext(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	t.Cleanup(cancel)
	return ctx
}
