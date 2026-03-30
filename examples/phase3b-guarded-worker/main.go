package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	goredis "github.com/redis/go-redis/v9"

	"lockman/guard"
	guardpostgres "lockman/guard/postgres"
	redisstore "lockman/idempotency/redis"
	"lockman/lockkit/definitions"
	redisdriver "lockman/lockkit/drivers/redis"
	"lockman/lockkit/registry"
	"lockman/lockkit/workers"
)

const (
	defaultRedisURL    = "redis://localhost:6379/0"
	defaultPostgresDSN = "postgres://postgres:postgres@localhost:5432/lockman?sslmode=disable"

	exampleDefinitionID = "StrictOrderClaim"
	exampleOrderID      = "order-123"
	exampleResourceKey  = "order:123"
)

type orderState struct {
	Status       string
	FencingToken uint64
	OwnerID      string
	LockID       string
}

type exampleOrderRepo struct {
	tableName string
}

func main() {
	redisURL := strings.TrimSpace(os.Getenv("LOCKMAN_REDIS_URL"))
	if redisURL == "" {
		redisURL = defaultRedisURL
	}

	postgresDSN := strings.TrimSpace(os.Getenv("LOCKMAN_POSTGRES_DSN"))
	if postgresDSN == "" {
		postgresDSN = defaultPostgresDSN
	}

	if err := run(os.Stdout, redisURL, postgresDSN); err != nil {
		fmt.Fprintf(os.Stderr, "example failed: %v\n", err)
		os.Exit(1)
	}
}

func run(out io.Writer, redisURL, postgresDSN string) error {
	client, err := newRedisClient(redisURL)
	if err != nil {
		return err
	}
	defer func() {
		_ = client.Close()
	}()

	db, err := openPostgres(postgresDSN)
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := exampleOrderRepo{
		tableName: fmt.Sprintf("phase3b_guarded_worker_orders_%d", time.Now().UnixNano()),
	}
	if err := repo.setupExampleOrder(ctx, db); err != nil {
		return err
	}
	defer func() {
		_ = repo.dropTable(context.Background(), db)
	}()

	prefix := fmt.Sprintf("lockman:example:phase3b:guarded-worker:%d", time.Now().UnixNano())
	driver := redisdriver.NewDriver(client, prefix+":lease")
	store := redisstore.New(client, prefix+":idempotency")

	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:                   exampleDefinitionID,
		Kind:                 definitions.KindParent,
		Resource:             "order",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionAsync,
		LeaseTTL:             5 * time.Second,
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		IdempotencyRequired:  true,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		return err
	}

	mgr, err := workers.NewManager(reg, driver, store)
	if err != nil {
		return err
	}

	firstReq := definitions.MessageClaimRequest{
		DefinitionID: exampleDefinitionID,
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "strict-worker-a",
			MessageID:     "strict-message-123-a",
			Attempt:       1,
			ConsumerGroup: "examples",
			HandlerName:   "Phase3bGuardedWorker",
		},
		IdempotencyKey: "strict-msg:123:a",
	}

	secondReq := definitions.MessageClaimRequest{
		DefinitionID: exampleDefinitionID,
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "strict-worker-b",
			MessageID:     "strict-message-123-b",
			Attempt:       1,
			ConsumerGroup: "examples",
			HandlerName:   "Phase3bGuardedWorker",
		},
		IdempotencyKey: "strict-msg:123:b",
	}

	var firstGuard guard.Context
	if err := mgr.ExecuteClaimed(ctx, firstReq, func(ctx context.Context, claim definitions.ClaimContext) error {
		firstGuard = guardContextFromClaim(claim)
		if _, err := fmt.Fprintf(out, "first worker claim token: %d\n", claim.FencingToken); err != nil {
			return err
		}

		outcome, err := repo.applyOrderStatus(ctx, db, firstGuard, exampleOrderID, "processing")
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "first guarded outcome: %s\n", outcome); err != nil {
			return err
		}
		return mapGuardOutcomeForWorker(outcome)
	}); err != nil {
		return err
	}

	if err := mgr.ExecuteClaimed(ctx, secondReq, func(ctx context.Context, claim definitions.ClaimContext) error {
		secondGuard := guardContextFromClaim(claim)
		if _, err := fmt.Fprintf(out, "second worker claim token: %d\n", claim.FencingToken); err != nil {
			return err
		}

		outcome, err := repo.applyOrderStatus(ctx, db, secondGuard, exampleOrderID, "completed")
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "second guarded outcome: %s\n", outcome); err != nil {
			return err
		}
		return mapGuardOutcomeForWorker(outcome)
	}); err != nil {
		return err
	}

	staleOutcome, err := repo.applyOrderStatus(ctx, db, firstGuard, exampleOrderID, "stale-write")
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "late stale outcome: %s\n", staleOutcome); err != nil {
		return err
	}

	state, err := repo.loadOrderState(ctx, db, exampleOrderID)
	if err != nil {
		return err
	}
	if state.Status != "completed" ||
		state.FencingToken != 2 ||
		state.OwnerID != secondReq.Ownership.OwnerID ||
		state.LockID != exampleDefinitionID {
		return fmt.Errorf(
			"unexpected final order state: status=%s token=%d owner=%s lock=%s",
			state.Status,
			state.FencingToken,
			state.OwnerID,
			state.LockID,
		)
	}

	record, err := store.Get(ctx, secondReq.IdempotencyKey)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "idempotency after ack: %s\n", record.Status); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, "teaching point: phase3b carries the strict fencing token into the database write path"); err != nil {
		return err
	}

	if err := mgr.Shutdown(context.Background()); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "shutdown: ok")
	return err
}

func (r exampleOrderRepo) applyOrderStatus(ctx context.Context, db *sql.DB, g guard.Context, orderID, status string) (guard.Outcome, error) {
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
`, r.tableName, r.tableName)

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

	updateStatus, err := guardpostgres.ScanExistingRowStatus(row)
	if err != nil {
		return "", err
	}

	return guardpostgres.ClassifyExistingRowUpdate(g, updateStatus)
}

func mapGuardOutcomeForWorker(outcome guard.Outcome) error {
	switch outcome {
	case guard.OutcomeApplied, guard.OutcomeStaleRejected, guard.OutcomeDuplicateIgnored:
		return nil
	default:
		return fmt.Errorf("%w: unsupported guard outcome %s", guard.ErrInvariantRejected, outcome)
	}
}

func guardContextFromClaim(claim definitions.ClaimContext) guard.Context {
	// Keep this mapping aligned with lockman/internal/guardbridge.FromClaimContext.
	// Examples should not import root-internal packages.
	return guard.Context{
		LockID:         claim.DefinitionID,
		ResourceKey:    claim.ResourceKey,
		FencingToken:   claim.FencingToken,
		OwnerID:        claim.Ownership.OwnerID,
		MessageID:      claim.Ownership.MessageID,
		IdempotencyKey: claim.IdempotencyKey,
	}
}

func newRedisClient(redisURL string) (goredis.UniversalClient, error) {
	if strings.TrimSpace(redisURL) == "" {
		return nil, fmt.Errorf("redis url is required")
	}

	opts, err := goredis.ParseURL(normalizeLoopbackURL(redisURL))
	if err != nil {
		return nil, err
	}

	return goredis.NewClient(opts), nil
}

func openPostgres(postgresDSN string) (*sql.DB, error) {
	if strings.TrimSpace(postgresDSN) == "" {
		return nil, fmt.Errorf("postgres dsn is required")
	}

	db, err := sql.Open("pgx", normalizeLoopbackURL(postgresDSN))
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func (r exampleOrderRepo) setupExampleOrder(ctx context.Context, db *sql.DB) error {
	createStmt := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
  id TEXT PRIMARY KEY,
  strict_lock_id TEXT NOT NULL,
  resource_key TEXT NOT NULL,
  status TEXT NOT NULL,
  last_fencing_token BIGINT NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_by_owner TEXT NOT NULL
)`, r.tableName)
	if _, err := db.ExecContext(ctx, createStmt); err != nil {
		return err
	}

	insertStmt := fmt.Sprintf(`
INSERT INTO %s (id, strict_lock_id, resource_key, status, last_fencing_token, updated_by_owner)
VALUES ($1, $2, $3, $4, $5, $6)
`, r.tableName)
	_, err := db.ExecContext(
		ctx,
		insertStmt,
		exampleOrderID,
		exampleDefinitionID,
		exampleResourceKey,
		"pending",
		int64(0),
		"seed",
	)
	return err
}

func (r exampleOrderRepo) loadOrderState(ctx context.Context, db *sql.DB, orderID string) (orderState, error) {
	query := fmt.Sprintf(`
SELECT status, last_fencing_token, updated_by_owner
  , strict_lock_id
FROM %s
WHERE id = $1
`, r.tableName)

	var state orderState
	var token int64
	if err := db.QueryRowContext(ctx, query, orderID).Scan(&state.Status, &token, &state.OwnerID, &state.LockID); err != nil {
		return orderState{}, err
	}
	state.FencingToken = uint64(token)
	return state, nil
}

func (r exampleOrderRepo) dropTable(ctx context.Context, db *sql.DB) error {
	dropStmt := fmt.Sprintf(`DROP TABLE IF EXISTS %s`, r.tableName)
	_, err := db.ExecContext(ctx, dropStmt)
	return err
}

func normalizeLoopbackURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.Hostname() != "localhost" {
		return raw
	}

	port := parsed.Port()
	if port == "" {
		parsed.Host = "127.0.0.1"
	} else {
		parsed.Host = net.JoinHostPort("127.0.0.1", port)
	}
	return parsed.String()
}
