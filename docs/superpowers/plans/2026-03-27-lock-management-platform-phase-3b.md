# Lock Management Platform Phase 3b Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Phase 3b guarded-write contracts and a first Postgres-backed persistence proof so strict workers can carry fencing tokens into database writes and stale writers are rejected at the storage boundary.

**Historical note (2026-03-30):** This document references `lockkit/guard` because that was the Phase 3b design at the time. The current stable guarded-write contract lives at the top-level `lockman/guard` package, and engine-to-contract mapping now lives behind a root-internal bridge.

**Architecture:** Keep the public API small and contract-driven. Add a new `lockkit/guard` package for shared context and outcomes, then add a narrow `lockkit/guard/postgres` helper layer that classifies the guarded single-row `UPDATE` query shape described in the Phase 3b spec without turning the SDK into a generic repository framework. Prove the worker-first path with one Redis + Postgres example and keep worker ack/retry policy outside the core lock runtime for this phase.

**Tech Stack:** Go 1.22+, standard library `testing`, `database/sql`, `github.com/jackc/pgx/v5/stdlib`, existing Redis driver and Redis idempotency store, local Docker Compose Redis/Postgres services

---

## Planned File Structure

### Existing files to extend

- `README.md`: add Phase 3b status, Postgres/Redis verification commands, and the new worker guarded-write example
- `docker-compose.yml`: add a local Postgres service alongside Redis for integration tests and examples
- `go.mod`: add the Postgres driver dependency used by `database/sql`
- `go.sum`: record checksums for the added Postgres driver dependencies
- `docs/lock-definition-reference.md`: update strict-mode field guidance so it points readers at Phase 3b guarded writes rather than stopping at Phase 3a fencing-token visibility

### New Phase 3b packages and files

- `lockkit/guard/context.go`: `guard.Context`, `guard.Outcome`, and `ContextFromLease` / `ContextFromClaim`
- `lockkit/guard/context_test.go`: unit tests for exact field mappings and zero-value runtime fields
- `lockkit/guard/postgres/existing_row.go`: the minimal Postgres adapter helper for scanning and classifying the guarded single-row `UPDATE` query shape
- `lockkit/guard/postgres/existing_row_test.go`: unit tests for `Applied`, `StaleRejected`, missing-row, and boundary-mismatch classification
- `lockkit/guard/postgres/existing_row_integration_test.go`: real-Postgres tests that execute the Phase 3b query pattern against a temporary table
- `examples/strict-guarded-write/main.go`: worker-first example using Redis strict execution plus Postgres guarded writes
- `examples/strict-guarded-write/main_test.go`: example output contract test
- `examples/strict-guarded-write/README.md`: example-specific prerequisites, run command, and output notes

## Phase Scope

This plan delivers only what the Phase 3b design requires:

- the new public `lockkit/guard` package
- a worker-first Postgres guarded-write proof for single-row updates
- missing-row and boundary-mismatch classification that does not collapse into stale-token handling
- docs and example coverage that show Phase 3b as the persistence half of strict mode

It does **not** implement:

- a generic repository framework
- automatic `guard.Outcome` to worker ack/retry/drop mapping inside `lockkit/workers`
- a new idempotency subsystem
- UPSERT support
- multi-statement guarded transactions
- strict composite or strict lineage guarded-write behavior

## Implementation Notes

- Use @superpowers:test-driven-development for every task. Write the failing test first, run it, then make the smallest implementation change that passes.
- Use @superpowers:verification-before-completion before claiming the phase is done.
- Use @superpowers:requesting-code-review after implementation tasks are complete and before merge.
- Keep `ContextFromLease(...)` and `ContextFromClaim(...)` as pure mapping helpers. Do not add policy or I/O to the `lockkit/guard` package.
- Keep the Postgres layer narrow. It should help classify the recommended guarded single-row `UPDATE` query shape, not become a generic repository command framework.
- Treat an equal fencing token as stale. The Postgres helper must use strict less-than (`<`) semantics and tests must prove that behavior.
- Missing-row and boundary-mismatch cases must not normalize to `OutcomeStaleRejected`.
- Do not modify `lockkit/workers` or `lockkit/internal/policy/outcome.go` to hard-code new worker policy in this phase. Any `mapGuardOutcomeForWorker(...)` helper used by the example should stay local to the example package.
- Redis-backed tests and examples should continue to skip when `LOCKMAN_REDIS_URL` is unset. Postgres-backed tests and examples should skip when `LOCKMAN_POSTGRES_DSN` is unset.
- Use `database/sql` plus `github.com/jackc/pgx/v5/stdlib`. Do not add an ORM.

### Task 1: Add The Public Guard Package

**Files:**
- Create: `lockkit/guard/context.go`
- Create: `lockkit/guard/context_test.go`

- [ ] **Step 1: Write the failing guard-context tests**

Add tests that pin the exact field mappings from the approved spec:

```go
func TestContextFromClaimMapsStrictWorkerFields(t *testing.T) {
	claim := definitions.ClaimContext{
		DefinitionID: "StrictOrderClaim",
		ResourceKey:  "order:123",
		Ownership: definitions.OwnershipMeta{
			OwnerID:   "worker-a",
			MessageID: "msg-123",
		},
		FencingToken:   7,
		IdempotencyKey: "idem-123",
	}

	got := guard.ContextFromClaim(claim)
	want := guard.Context{
		LockID:         "StrictOrderClaim",
		ResourceKey:    "order:123",
		FencingToken:   7,
		OwnerID:        "worker-a",
		MessageID:      "msg-123",
		IdempotencyKey: "idem-123",
	}

	if got != want {
		t.Fatalf("unexpected guard context: %#v", got)
	}
}

func TestContextFromLeaseLeavesWorkerOnlyFieldsZeroValue(t *testing.T) {
	lease := definitions.LeaseContext{
		DefinitionID: "StrictOrderLock",
		ResourceKey:  "order:123",
		Ownership: definitions.OwnershipMeta{
			OwnerID: "runtime-a",
		},
		FencingToken: 11,
	}

	got := guard.ContextFromLease(lease)
	if got.MessageID != "" || got.IdempotencyKey != "" {
		t.Fatalf("expected zero-value runtime-only fields, got %#v", got)
	}
}
```

Also add a small stability test that the exported outcome strings remain:

- `applied`
- `duplicate_ignored`
- `stale_rejected`
- `version_conflict`
- `invariant_rejected`

- [ ] **Step 2: Run the guard package tests to verify they fail**

Run: `go test ./lockkit/guard -v`
Expected: FAIL because the `lockkit/guard` package does not exist yet.

- [ ] **Step 3: Implement the public guard package**

Create `lockkit/guard/context.go` with:

```go
package guard

import "lockman/lockkit/definitions"

type Context struct {
	LockID         string
	ResourceKey    string
	FencingToken   uint64
	OwnerID        string
	MessageID      string
	IdempotencyKey string
}

type Outcome string

const (
	OutcomeApplied           Outcome = "applied"
	OutcomeDuplicateIgnored  Outcome = "duplicate_ignored"
	OutcomeStaleRejected     Outcome = "stale_rejected"
	OutcomeVersionConflict   Outcome = "version_conflict"
	OutcomeInvariantRejected Outcome = "invariant_rejected"
)

func ContextFromLease(lease definitions.LeaseContext) Context {
	return Context{
		LockID:       lease.DefinitionID,
		ResourceKey:  lease.ResourceKey,
		FencingToken: lease.FencingToken,
		OwnerID:      lease.Ownership.OwnerID,
	}
}

func ContextFromClaim(claim definitions.ClaimContext) Context {
	return Context{
		LockID:         claim.DefinitionID,
		ResourceKey:    claim.ResourceKey,
		FencingToken:   claim.FencingToken,
		OwnerID:        claim.Ownership.OwnerID,
		MessageID:      claim.Ownership.MessageID,
		IdempotencyKey: claim.IdempotencyKey,
	}
}
```

Do not add validation, repository interfaces, or policy helpers to this package in Phase 3b.

- [ ] **Step 4: Run the guard package tests to verify they pass**

Run: `go test ./lockkit/guard -v`
Expected: PASS

- [ ] **Step 5: Commit the guard package batch**

```bash
git add lockkit/guard/context.go lockkit/guard/context_test.go
git commit -m "feat: add guard context contracts"
```

### Task 2: Add The Narrow Postgres Guarded-Update Helper

**Files:**
- Create: `lockkit/guard/postgres/existing_row.go`
- Create: `lockkit/guard/postgres/existing_row_test.go`

- [ ] **Step 1: Write the failing Postgres helper unit tests**

Add tests that pin the non-stale classifications required by the spec:

```go
func TestClassifyExistingRowUpdateReturnsApplied(t *testing.T) {
	outcome, err := postgres.ClassifyExistingRowUpdate(
		guard.Context{LockID: "StrictOrderClaim", ResourceKey: "order:123", FencingToken: 5},
		postgres.ExistingRowStatus{Found: true, Applied: true, CurrentToken: 5, CurrentResourceKey: "order:123"},
	)
	if err != nil {
		t.Fatalf("ClassifyExistingRowUpdate returned error: %v", err)
	}
	if outcome != guard.OutcomeApplied {
		t.Fatalf("expected applied, got %q", outcome)
	}
}

func TestClassifyExistingRowUpdateTreatsEqualTokenAsStale(t *testing.T) {
	outcome, err := postgres.ClassifyExistingRowUpdate(
		guard.Context{LockID: "StrictOrderClaim", ResourceKey: "order:123", FencingToken: 5},
		postgres.ExistingRowStatus{Found: true, Applied: false, CurrentToken: 5, CurrentResourceKey: "order:123"},
	)
	if err != nil {
		t.Fatalf("ClassifyExistingRowUpdate returned error: %v", err)
	}
	if outcome != guard.OutcomeStaleRejected {
		t.Fatalf("expected stale, got %q", outcome)
	}
}

func TestClassifyExistingRowUpdateRejectsMissingRowAsInvariant(t *testing.T) {
	_, err := postgres.ClassifyExistingRowUpdate(
		guard.Context{LockID: "StrictOrderClaim", ResourceKey: "order:123", FencingToken: 5},
		postgres.ExistingRowStatus{Found: false},
	)
	if !errors.Is(err, lockerrors.ErrInvariantRejected) {
		t.Fatalf("expected invariant rejection, got %v", err)
	}
}

func TestClassifyExistingRowUpdateRejectsBoundaryMismatchAsInvariant(t *testing.T) {
	_, err := postgres.ClassifyExistingRowUpdate(
		guard.Context{LockID: "StrictOrderClaim", ResourceKey: "order:123", FencingToken: 5},
		postgres.ExistingRowStatus{Found: true, Applied: false, CurrentToken: 1, CurrentResourceKey: "order:456"},
	)
	if !errors.Is(err, lockerrors.ErrInvariantRejected) {
		t.Fatalf("expected invariant rejection, got %v", err)
	}
}
```

Also add a small test for the scanner helper:

```go
type stubScanner struct {
	values []any
	err    error
}
```

and verify `ScanExistingRowStatus(...)` decodes `found`, `applied`, `current_token`, and `current_resource_key` in that order.

- [ ] **Step 2: Run the Postgres helper unit tests to verify they fail**

Run: `go test ./lockkit/guard/postgres -run 'Classify|Scan' -v`
Expected: FAIL because the package and helper types do not exist yet.

- [ ] **Step 3: Implement the narrow Postgres helper**

Create `lockkit/guard/postgres/existing_row.go` with:

```go
package postgres

import (
	"fmt"

	"lockman/lockkit/guard"
	lockerrors "lockman/lockkit/errors"
)

type ExistingRowStatus struct {
	Found              bool
	Applied            bool
	CurrentToken       uint64
	CurrentResourceKey string
}

type rowScanner interface {
	Scan(dest ...any) error
}

func ScanExistingRowStatus(scanner rowScanner) (ExistingRowStatus, error) {
	var status ExistingRowStatus
	if err := scanner.Scan(
		&status.Found,
		&status.Applied,
		&status.CurrentToken,
		&status.CurrentResourceKey,
	); err != nil {
		return ExistingRowStatus{}, err
	}
	return status, nil
}

func ClassifyExistingRowUpdate(g guard.Context, status ExistingRowStatus) (guard.Outcome, error) {
	switch {
	case !status.Found:
		return "", fmt.Errorf("%w: guarded row not found for %s", lockerrors.ErrInvariantRejected, g.ResourceKey)
	case status.Applied:
		return guard.OutcomeApplied, nil
	case status.CurrentResourceKey != g.ResourceKey:
		return "", fmt.Errorf("%w: guarded boundary mismatch want=%s got=%s", lockerrors.ErrInvariantRejected, g.ResourceKey, status.CurrentResourceKey)
	case status.CurrentToken >= g.FencingToken:
		return guard.OutcomeStaleRejected, nil
	default:
		return "", fmt.Errorf("%w: inconsistent guarded update state", lockerrors.ErrInvariantRejected)
	}
}
```

Keep this package narrow. Do not introduce a generic repository interface, command framework, or multi-row abstraction.

- [ ] **Step 4: Run the Postgres helper unit tests to verify they pass**

Run: `go test ./lockkit/guard/postgres -run 'Classify|Scan' -v`
Expected: PASS

- [ ] **Step 5: Commit the Postgres helper batch**

```bash
git add lockkit/guard/postgres/existing_row.go lockkit/guard/postgres/existing_row_test.go
git commit -m "feat: add postgres guarded update classifier"
```

### Task 3: Add Real Postgres Integration Coverage And Dev Support

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `docker-compose.yml`
- Create: `lockkit/guard/postgres/existing_row_integration_test.go`

- [ ] **Step 1: Write the failing Postgres integration tests**

Add integration tests that exercise the actual SQL query pattern from the approved spec.

Use a helper like:

```go
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

	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("PingContext returned error: %v", err)
	}
	return db
}
```

Create a temporary `orders` table and write tests proving:

- a newer token updates the row and classifies as `OutcomeApplied`
- an older token classifies as `OutcomeStaleRejected`
- an equal token classifies as `OutcomeStaleRejected`
- a missing row returns `lockerrors.ErrInvariantRejected`
- a boundary mismatch returns `lockerrors.ErrInvariantRejected`

Use the exact query shape the spec approved:

```sql
WITH target AS (
  SELECT id, resource_key, last_fencing_token
  FROM orders
  WHERE id = $4
),
updated AS (
  UPDATE orders
  SET
    status = $1,
    last_fencing_token = $2,
    updated_at = NOW(),
    updated_by_owner = $3
  WHERE id = $4
    AND resource_key = $5
    AND last_fencing_token < $2
  RETURNING id
)
SELECT
  EXISTS(SELECT 1 FROM target) AS found,
  EXISTS(SELECT 1 FROM updated) AS applied,
  COALESCE((SELECT last_fencing_token FROM target), 0) AS current_token,
  COALESCE((SELECT resource_key FROM target), '') AS current_resource_key
```

- [ ] **Step 2: Run the Postgres integration tests to verify they fail**

Run: `LOCKMAN_POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/lockman?sslmode=disable go test ./lockkit/guard/postgres -run 'Integration' -v`
Expected: FAIL because the Postgres driver dependency, integration file, and/or local Postgres service are not ready yet.

- [ ] **Step 3: Add Postgres dependency and local Docker Compose support**

Update `go.mod` and `go.sum` to add:

```go
go get github.com/jackc/pgx/v5@latest
```

Then import `github.com/jackc/pgx/v5/stdlib` from the integration test so `database/sql` can open the `pgx` driver.

Extend `docker-compose.yml` with:

```yaml
  postgres:
    image: postgres:16-alpine
    container_name: lockman-postgres
    environment:
      POSTGRES_DB: lockman
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
    ports:
      - "${LOCKMAN_POSTGRES_PORT:-5432}:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U postgres -d lockman"]
      interval: 5s
      timeout: 3s
      retries: 10
```

- [ ] **Step 4: Implement the integration test file**

Create `lockkit/guard/postgres/existing_row_integration_test.go` using `database/sql` and the `pgx` driver.

Use a table name suffixed with `time.Now().UnixNano()` per test so isolation matches the existing Redis-prefix pattern used by the Phase 3a examples. Do not use `TRUNCATE` as the primary isolation mechanism, and do not depend on pre-existing schema outside the test file.

- [ ] **Step 5: Run the Postgres integration tests to verify they pass**

Run:

```bash
docker compose up -d postgres
LOCKMAN_POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/lockman?sslmode=disable go test ./lockkit/guard/postgres -run 'Integration' -v
```

Expected: PASS

- [ ] **Step 6: Commit the integration-support batch**

```bash
git add go.mod go.sum docker-compose.yml lockkit/guard/postgres/existing_row_integration_test.go
git commit -m "test: add postgres guarded update integration coverage"
```

### Task 4: Add The Worker-First Guarded-Write Example

**Files:**
- Create: `examples/strict-guarded-write/main.go`
- Create: `examples/strict-guarded-write/main_test.go`
- Create: `examples/strict-guarded-write/README.md`

- [ ] **Step 1: Write the failing example test**

Create `examples/strict-guarded-write/main_test.go` with a skip gate for both Redis and Postgres:

```go
func TestRunPrintsGuardedWorkerFlow(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("LOCKMAN_REDIS_URL"))
	if redisURL == "" {
		t.Skip("LOCKMAN_REDIS_URL is not set")
	}
	postgresDSN := strings.TrimSpace(os.Getenv("LOCKMAN_POSTGRES_DSN"))
	if postgresDSN == "" {
		t.Skip("LOCKMAN_POSTGRES_DSN is not set")
	}

	var out bytes.Buffer
	if err := run(&out, redisURL, postgresDSN); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	expected := []string{
		"first worker claim token: 1",
		"first guarded outcome: applied",
		"second worker claim token: 2",
		"second guarded outcome: applied",
		"late stale outcome: stale_rejected",
		"idempotency after ack: completed",
		"teaching point: phase3b carries the strict fencing token into the database write path",
		"shutdown: ok",
	}

	output := out.String()
	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
```

- [ ] **Step 2: Run the example test to verify it fails**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 LOCKMAN_POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/lockman?sslmode=disable go test ./examples/strict-guarded-write -v`
Expected: FAIL because `main.go` and `run(...)` do not exist yet.

- [ ] **Step 3: Implement the example**

Create `examples/strict-guarded-write/main.go` with:

- a `run(out io.Writer, redisURL, postgresDSN string) error` entry point
- a strict async definition registered with `workers.NewManager(...)`
- a small Postgres setup helper that creates and seeds one `orders` row for the example run
- a domain-specific repository method such as:

```go
func applyOrderStatus(ctx context.Context, db *sql.DB, g guard.Context, orderID, status string) (guard.Outcome, error)
```

Inside `applyOrderStatus(...)`:

1. execute the approved guarded `UPDATE` query
2. call `postgres.ScanExistingRowStatus(...)`
3. call `postgres.ClassifyExistingRowUpdate(...)`
4. return the resulting `guard.Outcome`

Use two separate strict worker requests on the same order boundary:

- request 1 gets token `1` and applies the first update
- request 2 gets token `2` and applies the second update
- after request 2 commits, reuse the captured guard context from request 1 to attempt one late write and show `stale_rejected`

The two requests must keep the same lock boundary (`DefinitionID` + `KeyInput`) but use distinct worker-delivery metadata so the second callback actually runs:

- request 1 and request 2 must use different `IdempotencyKey` values
- request 1 and request 2 must use different `Ownership.MessageID` values
- request 1 and request 2 should use different `Ownership.OwnerID` values so the example makes the stale-writer handoff obvious in logs

The late stale attempt must call `applyOrderStatus(...)` directly against the database, outside any `ExecuteClaimed(...)` callback, using the guard context captured from the first worker claim. This keeps the stale-write demo separate from worker idempotency terminal-state handling and preserves the expected `idempotency after ack: completed` output for the successful second claim.

Keep `mapGuardOutcomeForWorker(...)` local to the example package if you need it. Do not modify the SDK worker runtime in this phase.

Inside the example-local mapper:

- `guard.OutcomeApplied` should map to `nil`
- `guard.OutcomeStaleRejected` should map to `nil`
- `guard.OutcomeDuplicateIgnored` should map to `nil`
- other outcomes may map to domain-specific errors

In other words, the example should treat `OutcomeStaleRejected` as a terminal ACK-style business result, not as `lockerrors.ErrInvariantRejected` or any other retry/drop-triggering worker error.

- [ ] **Step 4: Write the example README**

Create `examples/strict-guarded-write/README.md` documenting:

- Redis and Postgres prerequisites
- `LOCKMAN_REDIS_URL`
- `LOCKMAN_POSTGRES_DSN`
- the run command
- the meaning of the output lines

Use this run command:

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 \
LOCKMAN_POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/lockman?sslmode=disable \
go run ./examples/strict-guarded-write
```

- [ ] **Step 5: Run the example test to verify it passes**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 LOCKMAN_POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/lockman?sslmode=disable go test ./examples/strict-guarded-write -v`
Expected: PASS

- [ ] **Step 6: Commit the example batch**

```bash
git add examples/strict-guarded-write/main.go examples/strict-guarded-write/main_test.go examples/strict-guarded-write/README.md
git commit -m "feat: add phase 3b guarded worker example"
```

### Task 5: Update Phase Docs And Run Full Verification

**Files:**
- Modify: `README.md`
- Modify: `docs/lock-definition-reference.md`

- [ ] **Step 1: Update README and strict-mode reference docs**

Update `README.md` to:

- add a `Phase 3b Status` section
- document that guarded-write persistence safety now exists through `lockkit/guard` and the Postgres example path
- document `LOCKMAN_POSTGRES_DSN` next to the existing Redis verification commands
- add `go run ./examples/strict-guarded-write` to the command list
- add the new example README link near the Phase 3a strict examples

Update `docs/lock-definition-reference.md` to:

- change strict-mode guidance so it no longer stops at “guarded writes remain out of scope”
- call out that Phase 3b adds guarded-write contracts and a first Postgres-backed persistence proof
- keep strict composite / strict lineage caveats out of scope

- [ ] **Step 2: Run focused package and example verification**

Run:

```bash
LOCKMAN_POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/lockman?sslmode=disable \
go test ./lockkit/guard ./lockkit/guard/postgres -v
```

Expected: PASS

Run:

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 \
LOCKMAN_POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/lockman?sslmode=disable \
go test ./examples/strict-guarded-write -v
```

Expected: PASS

- [ ] **Step 3: Run broader verification before completion**

Run:

```bash
docker compose up -d redis postgres
LOCKMAN_REDIS_URL=redis://localhost:6379/0 \
LOCKMAN_POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/lockman?sslmode=disable \
go test ./...
```

Expected: PASS, with Redis-backed and Postgres-backed integration tests/examples running against the local Compose services.

- [ ] **Step 4: Commit the documentation and verification batch**

```bash
git add README.md docs/lock-definition-reference.md
git commit -m "docs: add phase 3b guarded write references"
```

- [ ] **Step 5: Final verification notes**

Record in the execution handoff:

- whether Postgres integration tests ran or skipped
- the exact `LOCKMAN_POSTGRES_DSN` shape used for local verification
- that `lockkit/workers` policy mapping was intentionally left unchanged in Phase 3b
