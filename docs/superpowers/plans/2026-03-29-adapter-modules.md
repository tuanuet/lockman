# Adapter Modules Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract concrete Redis and Postgres adapters into separate Go modules, promote stable contracts to top-level packages, and keep the root `lockman` SDK and released root module free from `lockkit` adapter imports.

**Architecture:** First promote stable contract packages (`backend`, `idempotency`, `guard`) and rewire the root SDK plus engine to use them. Then create nested adapter modules wired by `go.work`, migrate runnable adapter-dependent examples into those modules, and finish with root-module `GOWORK=off` validation so published-root behavior does not depend on sibling modules being present.

**Tech Stack:** Go 1.22, Go workspaces (`go.work`), existing `lockman` root SDK, existing `lockkit` engine, `go-redis`, `pgx`, standard library `testing`

---

## Planned File Structure

### Existing files to modify

- `go.mod`: drop root dependencies that become adapter-only after extraction
- `README.md`: retarget adapter documentation and remove runnable root examples that require sibling modules
- `identity.go`: switch `WithBackend(...)` and `WithIdempotency(...)` to top-level contracts
- `client.go`: switch `Client` fields and manager wiring to top-level contracts
- `client_test.go`: update stubs and expectations to top-level contracts
- `client_run_test.go`: keep runtime coverage after contract rewiring
- `client_claim_test.go`: keep worker coverage after contract rewiring
- `client_validation.go`: detect strict and lineage capabilities via top-level backend contracts
- `errors.go`: keep public SDK error mapping stable while backend contract errors move out of `lockkit`
- `redis/doc.go`: rewrite package comment for nested-module reality or replace with nested-module-local doc
- `redis/backend.go`: replace wrapper implementation with real adapter-module entry point
- `redis/backend_test.go`: test the adapter module directly against promoted contracts
- `idempotency/redis/doc.go`: rewrite package comment for nested-module reality
- `idempotency/redis/store.go`: replace wrapper implementation with real adapter-module entry point
- `advanced/guard/doc.go`: clarify distinction from top-level `guard` contract package
- `docs/advanced/guard.md`: clarify distinction from top-level `guard` contract package
- `docs/errors.md`: ensure docs do not point readers toward old adapter internals
- `docs/quickstart-sync.md`: keep snippet-only adapter imports, no runnable root example dependency
- `docs/quickstart-async.md`: keep snippet-only adapter imports, no runnable root example dependency
- `docs/registry-and-usecases.md`: keep adapter guidance aligned with extracted modules
- `examples/sync-approve-order/main.go`: either retire from root or move into adapter module
- `examples/sync-approve-order/main_test.go`: retire or move with the example
- `examples/async-process-order/main.go`: retire from root or move into adapter module
- `examples/async-process-order/main_test.go`: retire or move with the example
- `examples/sync-transfer-funds/main.go`: retire from root or move into adapter module
- `examples/sync-transfer-funds/main_test.go`: retire or move with the example
- `examples/sync-fenced-write/main.go`: retire from root or move into adapter module
- `examples/sync-fenced-write/main_test.go`: retire or move with the example
- `examples/async-single-resource/main.go`: retarget import path or explicitly archive as historical
- `examples/async-bulk-import-shard/main.go`: retarget import path or explicitly archive as historical
- `examples/async-composite-lock/main.go`: retarget import path or explicitly archive as historical
- `examples/shared-aggregate-split-definitions/main.go`: retarget import path or explicitly archive as historical
- `examples/shared-definition-contention/main.go`: retarget import path or explicitly archive as historical
- `examples/strict-async-fencing/main.go`: retarget import path or explicitly archive as historical
- `examples/strict-guarded-write/main.go`: retarget to top-level `guard` and `guard/postgres`
- `lockkit/runtime/manager.go`: import promoted backend contracts
- `lockkit/runtime/exclusive.go`: import promoted backend contracts and promoted backend errors
- `lockkit/runtime/composite.go`: import promoted backend contracts
- `lockkit/runtime/presence.go`: import promoted backend contracts
- `lockkit/workers/manager.go`: import promoted backend and idempotency contracts
- `lockkit/workers/execute.go`: import promoted backend and idempotency contracts
- `lockkit/workers/execute_composite.go`: import promoted backend and idempotency contracts
- `lockkit/workers/renewal.go`: import promoted backend contracts
- `lockkit/testkit/memory_driver.go`: satisfy promoted backend contracts
- `lockkit/testkit/memory_driver_test.go`: update imports/types if needed
- `lockkit/internal/policy/outcome.go`: map promoted backend errors rather than `lockkit/errors`
- `lockkit/drivers/contracts.go`: delete or leave as a temporary compatibility shim only if the implementation batch requires it during migration; end state should remove it from the root API graph
- `lockkit/idempotency/contracts.go`: same note as above; end state should no longer source root contracts from here
- `lockkit/idempotency/memory_store.go`: move `NewMemoryStore` out
- `lockkit/guard/context.go`: move top-level stable types out, leave only internal bridge if still needed
- `lockkit/guard/postgres/existing_row.go`: move into nested module and retarget imports
- `lockkit/guard/postgres/existing_row_test.go`: move with adapter module
- `lockkit/guard/postgres/existing_row_integration_test.go`: move with adapter module

### New root-level contract files

- `backend/contracts.go`: promoted driver contracts, optional capabilities, shared request/record types, lineage kind type, backend sentinel errors
- `backend/contracts_test.go`: unit tests for sentinel stability and contract helpers
- `idempotency/contracts.go`: promoted store contracts and record/begin/complete/fail types
- `idempotency/memory_store.go`: promoted in-memory supported store
- `idempotency/memory_store_test.go`: move existing memory store tests here
- `guard/context.go`: top-level stable `Context`, `Outcome`, and guard-scoped errors only
- `guard/context_test.go`: tests for stable contract values if needed

### New root-internal bridge/support files

- `internal/guardbridge/from_engine.go`: bridge from internal engine lease/claim contexts to top-level `guard.Context`
- `internal/guardbridge/from_engine_test.go`: tests for exact field mapping from engine contexts
- `tools.go` or `go.work`: workspace wiring if `go.work` is not sufficient alone for documentation of workspace usage

### New nested-module files

- `go.work`: workspace including root module plus adapter modules
- `redis/go.mod`
- `redis/go.sum`
- `redis/backend.go`: real Redis backend implementation moved from `lockkit/drivers/redis`
- `redis/backend_test.go`: migrated adapter tests
- `redis/doc.go`
- `redis/scripts.go`: migrated Redis scripts/helpers if currently separate under `lockkit`
- `redis/integration_test.go`: migrated or renamed integration tests
- `idempotency/redis/go.mod`
- `idempotency/redis/go.sum`
- `idempotency/redis/store.go`: real Redis idempotency implementation moved from `lockkit/idempotency/redis`
- `idempotency/redis/store_test.go`: migrated unit/integration tests
- `guard/postgres/go.mod`
- `guard/postgres/go.sum`
- `guard/postgres/existing_row.go`: migrated Postgres guarded-write helper
- `guard/postgres/existing_row_test.go`
- `guard/postgres/existing_row_integration_test.go`

### New adapter-module runnable examples

- `backend/redis/examples/sync-approve-order/main.go`
- `backend/redis/examples/sync-approve-order/main_test.go`
- `backend/redis/examples/sync-transfer-funds/main.go`
- `backend/redis/examples/sync-transfer-funds/main_test.go`
- `backend/redis/examples/sync-fenced-write/main.go`
- `backend/redis/examples/sync-fenced-write/main_test.go`
- `idempotency/redis/examples/async-process-order/main.go`
- `idempotency/redis/examples/async-process-order/main_test.go`

## Task 1: Promote Stable Backend Contracts

**Files:**
- Create: `backend/contracts.go`
- Create: `backend/contracts_test.go`
- Modify: `identity.go`
- Modify: `client.go`
- Modify: `client_run_test.go`
- Modify: `client_test.go`
- Modify: `client_validation.go`
- Modify: `errors.go`
- Modify or delete: `lockkit/drivers/contracts.go`
- Modify: `lockkit/runtime/manager.go`
- Modify: `lockkit/runtime/exclusive.go`
- Modify: `lockkit/runtime/composite.go`
- Modify: `lockkit/runtime/presence.go`
- Modify: `lockkit/workers/manager.go`
- Modify: `lockkit/workers/execute.go`
- Modify: `lockkit/workers/execute_composite.go`
- Modify: `lockkit/workers/renewal.go`
- Modify: `lockkit/testkit/memory_driver.go`
- Modify: `lockkit/internal/policy/outcome.go`

- [ ] **Step 1: Write the failing backend contract tests**

Create `backend/contracts_test.go` to pin:

- sentinel error identity survives `errors.Is`
- promoted lineage kind type replaces direct `lockkit/definitions.LockKind` in the backend contract
- promoted interfaces still permit strict and lineage capability detection

- [ ] **Step 2: Run the focused backend contract test to verify it fails**

Run: `go test ./backend -run 'Sentinel|Lineage|Capability' -v`
Expected: FAIL because `backend` package does not exist yet.

- [ ] **Step 3: Implement `backend/contracts.go`**

Promote from current `lockkit/drivers/contracts.go`:

- `Driver`
- `StrictDriver`
- `LineageDriver`
- acquire/presence request and record types
- backend sentinel errors like lease-held / not-found / owner-mismatch
- backend-scoped lineage kind type replacing `definitions.LockKind` in the contract

- [ ] **Step 4: Repoint root SDK and engine imports to `backend`**

Update:

- `identity.go`
- `client.go`
- `client_validation.go`
- `errors.go`
- runtime/worker manager and execution files
- `lockkit/testkit/memory_driver.go`
- `lockkit/internal/policy/outcome.go`

Do not leave root SDK types depending on `lockkit/drivers`.
Resolve `lockkit/drivers/contracts.go` in this task by deleting it or reducing it to a temporary compatibility shim that is no longer part of the supported root API graph.

- [ ] **Step 5: Run focused tests**

Run: `go test ./backend ./lockkit/runtime ./lockkit/workers ./lockkit/testkit -v`
Expected: PASS

- [ ] **Step 6: Run targeted root client tests**

Run: `go test ./ -run 'Client|Run|MapEngineError' -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add backend identity.go client.go client_run_test.go client_test.go client_validation.go errors.go lockkit/drivers/contracts.go lockkit/runtime lockkit/workers lockkit/testkit lockkit/internal/policy
git commit -m "refactor: promote backend contracts"
```

## Task 2: Promote Stable Idempotency Contracts And Memory Store

**Files:**
- Create: `idempotency/contracts.go`
- Create: `idempotency/memory_store.go`
- Create: `idempotency/memory_store_test.go`
- Modify: `identity.go`
- Modify: `client.go`
- Modify: `client_claim_test.go`
- Modify: `client_test.go`
- Modify: `lockkit/workers/manager.go`
- Modify: `lockkit/workers/execute.go`
- Modify: `lockkit/workers/execute_composite.go`
- Delete or stop importing: `lockkit/idempotency/contracts.go`
- Delete or stop importing: `lockkit/idempotency/memory_store.go`

- [ ] **Step 1: Write the failing top-level idempotency tests**

Create tests pinning:

- top-level `idempotency.Store` contract exists
- `idempotency.NewMemoryStore()` exists at the top level
- root client and worker code can compile against the promoted contract

- [ ] **Step 2: Run the focused idempotency test to verify it fails**

Run: `go test ./idempotency -run 'MemoryStore|StoreContract' -v`
Expected: FAIL because the top-level contract package is incomplete.

- [ ] **Step 3: Implement promoted idempotency contract and memory store**

Move the current contract types and in-memory implementation into top-level `idempotency`.

- [ ] **Step 4: Repoint root SDK and workers to top-level `idempotency`**

Update:

- `identity.go`
- `client.go`
- root client tests
- worker manager and execute files

- [ ] **Step 5: Run focused tests**

Run: `go test ./idempotency ./lockkit/workers ./ -run 'Claim|Client' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add idempotency identity.go client.go client_claim_test.go client_test.go lockkit/workers
git commit -m "refactor: promote idempotency contracts"
```

## Task 3: Promote Stable Guard Contract And Add Internal Bridge

**Files:**
- Create: `guard/context.go`
- Create: `guard/context_test.go`
- Create: `internal/guardbridge/from_engine.go`
- Create: `internal/guardbridge/from_engine_test.go`
- Modify: `advanced/guard/doc.go`
- Modify: `docs/advanced/guard.md`
- Delete or stop importing: `lockkit/guard/context.go`

- [ ] **Step 1: Write the failing guard contract and bridge tests**

Create tests that pin:

- top-level `guard.Context` and `guard.Outcome`
- no top-level `guard.ContextFromLease(...)` or `guard.ContextFromClaim(...)`
- internal bridge maps exact fields from internal engine contexts into `guard.Context`

- [ ] **Step 2: Run the focused guard tests to verify they fail**

Run: `go test ./guard ./internal/guardbridge -v`
Expected: FAIL because packages do not exist yet.

- [ ] **Step 3: Implement top-level `guard` stable data types**

Move only:

- `Context`
- `Outcome`
- any guard-scoped sentinel errors needed by guard adapter modules

Do not export lease/claim mapping helpers from this top-level package.

- [ ] **Step 4: Implement internal bridge from engine contexts**

Create `internal/guardbridge/from_engine.go` that converts internal engine lease/claim contexts into top-level `guard.Context`.

- [ ] **Step 5: Update docs to distinguish `guard` contract from `advanced/guard`**

Clarify:

- `guard`: low-level contract package
- `advanced/guard`: advanced namespace/docs

- [ ] **Step 6: Run focused tests**

Run: `go test ./guard ./internal/guardbridge -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add guard internal/guardbridge advanced/guard/doc.go docs/advanced/guard.md
git commit -m "refactor: add guard contract and internal bridge"
```

## Task 4: Add `go.work` And Extract Redis Backend Module

**Files:**
- Create: `go.work`
- Create: `redis/go.mod`
- Create: `redis/go.sum`
- Modify: `redis/backend.go`
- Modify: `redis/backend_test.go`
- Modify: `redis/doc.go`
- Move/modify: implementation files from `lockkit/drivers/redis/*`
- Modify: `go.mod`

- [ ] **Step 1: Write the failing workspace and Redis adapter tests**

Add or adapt tests so `redis` module builds against `backend.Driver` and strict/lineage capabilities through its own `go.mod`.

- [ ] **Step 2: Add `go.work`**

Create `go.work` with:

```text
use (
	.
	./backend/redis
)
```

- [ ] **Step 3: Create `redis/go.mod` and move Redis backend implementation**

Create `redis/go.mod` with `module lockman/backend/redis` (matching the root module path pattern) and move the concrete implementation from `lockkit/drivers/redis` into the `redis` module so `lockman/backend/redis` is the real adapter package.

- [ ] **Step 4: Repoint imports to top-level `backend` and promoted shared types**

Ensure the Redis backend no longer imports any `lockkit/*` package.

- [ ] **Step 5: Run import-boundary verification for the Redis module**

Run: `rg -n 'lockkit/' redis`
Expected: no matches in supported Redis adapter code.

- [ ] **Step 6: Drop root-only dependency if possible**

If root no longer needs `go-redis` for compilation after runnable examples move out, remove it later in the dependency cleanup task. For now keep the root tidy but compiling.

- [ ] **Step 7: Run focused Redis module tests**

Run: `cd backend/redis && go test ./... -v`
Expected: PASS

- [ ] **Step 8: Run workspace verification for root + Redis**

Run:

```bash
go test ./...
cd backend/redis && go test ./... -v
```

Expected: PASS for the root module plus the Redis module.

- [ ] **Step 9: Commit**

```bash
git add go.work redis go.mod
git commit -m "refactor: extract redis backend module"
```

## Task 5: Extract Redis Idempotency Module

**Files:**
- Create: `idempotency/redis/go.mod`
- Create: `idempotency/redis/go.sum`
- Modify: `idempotency/redis/store.go`
- Modify: `idempotency/redis/doc.go`
- Move/modify: implementation files from `lockkit/idempotency/redis/*`

- [ ] **Step 1: Write the failing Redis idempotency module tests**

Adapt tests so the module builds against top-level `idempotency.Store`.

- [ ] **Step 2: Create `idempotency/redis/go.mod`**

Wire the new module into `go.work` by updating it to:

```text
use (
	.
	./backend/redis
	./idempotency/redis
)
```

Set the module path to `lockman/idempotency/redis`.

- [ ] **Step 3: Move the concrete implementation**

Move the Redis idempotency implementation out of `lockkit/idempotency/redis` into `idempotency/redis`.

- [ ] **Step 4: Remove `lockkit/*` imports from the module**

Ensure the adapter uses top-level `idempotency` contracts only.

- [ ] **Step 5: Run import-boundary verification for the Redis idempotency module**

Run: `rg -n 'lockkit/' idempotency/redis`
Expected: no matches in supported Redis idempotency adapter code.

- [ ] **Step 6: Run focused tests**

Run: `cd idempotency/redis && go test ./... -v`
Expected: PASS

- [ ] **Step 7: Run workspace verification**

Run:

```bash
go test ./...
cd backend/redis && go test ./... -v
cd ../idempotency/redis && go test ./... -v
```

Expected: PASS for root plus both existing adapter modules.

- [ ] **Step 8: Commit**

```bash
git add idempotency/redis go.work
git commit -m "refactor: extract redis idempotency module"
```

## Task 6: Extract Postgres Guard Module

**Files:**
- Create: `guard/postgres/go.mod`
- Create: `guard/postgres/go.sum`
- Move/modify: `lockkit/guard/postgres/existing_row.go`
- Move/modify: `lockkit/guard/postgres/existing_row_test.go`
- Move/modify: `lockkit/guard/postgres/existing_row_integration_test.go`
- Modify: `examples/strict-guarded-write/main.go`

- [ ] **Step 1: Write the failing Postgres guard module tests**

Pin that the module compiles against top-level `guard` contract types and guard-scoped sentinel errors.

- [ ] **Step 2: Create `guard/postgres/go.mod`**

Wire the new module into `go.work` by updating it to:

```text
use (
	.
	./backend/redis
	./idempotency/redis
	./guard/postgres
)
```

Set the module path to `lockman/guard/postgres`.

- [ ] **Step 3: Move the implementation**

Move the Postgres helper into `guard/postgres`.

- [ ] **Step 4: Remove `lockkit/errors` and `lockkit/guard` imports**

Use top-level `guard` contract types and guard-scoped or backend-scoped shared errors as defined by the spec.

- [ ] **Step 5: Run import-boundary verification for the Postgres guard module**

Run: `rg -n 'lockkit/' guard/postgres`
Expected: no matches in supported Postgres guard adapter code.

- [ ] **Step 6: Update the guarded worker example**

Repoint `examples/strict-guarded-write/main.go` to top-level `guard` and nested-module `guard/postgres`, or archive/move the runnable example if root release behavior must stay `GOWORK=off` clean.

- [ ] **Step 7: Run focused tests**

Run:

```bash
go test ./...
cd backend/redis && go test ./... -v
cd ../idempotency/redis && go test ./... -v
cd ../../guard/postgres && go test ./... -v
```

Expected: PASS for root plus all adapter modules created so far.

- [ ] **Step 8: Commit**

```bash
git add guard/postgres examples/strict-guarded-write
git commit -m "refactor: extract postgres guard module"
```

## Task 7: Move Runnable Adapter-Dependent Examples Out Of Root

**Files:**
- Create: `backend/redis/examples/sync-approve-order/main.go`
- Create: `backend/redis/examples/sync-approve-order/main_test.go`
- Create: `backend/redis/examples/sync-transfer-funds/main.go`
- Create: `backend/redis/examples/sync-transfer-funds/main_test.go`
- Create: `backend/redis/examples/sync-fenced-write/main.go`
- Create: `backend/redis/examples/sync-fenced-write/main_test.go`
- Create: `idempotency/redis/examples/async-process-order/main.go`
- Create: `idempotency/redis/examples/async-process-order/main_test.go`
- Modify or retire: root runnable adapter-dependent examples
- Modify: `README.md`
- Modify: `docs/quickstart-sync.md`
- Modify: `docs/quickstart-async.md`
- Modify: `docs/errors.md`
- Modify: `docs/registry-and-usecases.md`

- [ ] **Step 1: Move supported runnable examples into adapter modules**

Keep teaching value, but relocate runnable packages so the root module can verify with `GOWORK=off`.

- [ ] **Step 2: Retarget README and quickstarts**

Update doc links and run commands to point to the new adapter-module example paths.

- [ ] **Step 3: Decide root-example treatment explicitly**

Either:

- delete adapter-dependent root runnable examples, or
- leave only non-runnable snippet documentation in root

Do not leave root `go test ./...` dependent on sibling modules.

- [ ] **Step 4: Run root-module released behavior check**

Run: `GOWORK=off go test ./...`
Expected: PASS from the root module alone after runnable adapter-dependent examples are moved or retired.

- [ ] **Step 5: Run adapter-module example tests**

Run:

```bash
cd backend/redis && go test ./examples/... -v
cd ../idempotency/redis && go test ./examples/... -v
```

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add README.md docs redis/examples idempotency/redis/examples examples
git commit -m "refactor: move adapter examples out of root"
```

## Task 8: Historical Example Triage And Import Cleanup

**Files:**
- Modify or archive: historical `examples/phase*` packages that still import low-level adapter packages
- Modify docs that still mention old adapter internals

- [ ] **Step 1: Inventory remaining `lockkit` adapter imports**

Run: `rg -n 'lockkit/(drivers/redis|idempotency/redis|guard/postgres)' .`
Expected: list of remaining historical or accidental imports.

- [ ] **Step 2: Classify each remaining use**

For each hit, choose exactly one:

- migrate to new adapter module path
- archive as historical/internal

- [ ] **Step 3: Implement the classification**

Do not leave mixed or half-migrated examples behind.

- [ ] **Step 4: Verify no supported surface imports old adapter paths**

Run:

```bash
rg -n 'lockkit/(drivers/redis|idempotency/redis|guard/postgres)' README.md docs advanced examples
```

Expected: only explicitly archived historical references remain, or no matches if archives are moved out of the supported tree.

- [ ] **Step 5: Run released-root verification after historical cleanup**

Run: `GOWORK=off go test ./...`
Expected: PASS from the root module alone.

- [ ] **Step 6: Commit**

```bash
git add README.md docs examples
git commit -m "refactor: clean up historical adapter imports"
```

## Task 9: Final Dependency And Verification Sweep

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Modify: `go.work`
- Modify: any module-specific `go.mod`/`go.sum`

- [ ] **Step 1: Remove adapter-only root dependencies where now possible**

Drop root `go-redis` and `pgx` dependencies if the root module no longer compiles against adapter-specific code.

- [ ] **Step 2: Sync module metadata**

Run:

```bash
go work sync
go mod tidy
cd backend/redis && go mod tidy
cd ../idempotency/redis && go mod tidy
cd ../../guard/postgres && go mod tidy
```

- [ ] **Step 3: Run full workspace verification**

Run:

```bash
go test ./...
cd backend/redis && go test ./... 
cd ../idempotency/redis && go test ./...
cd ../../guard/postgres && go test ./...
```

Expected: PASS in workspace mode.

- [ ] **Step 4: Run released-root verification**

Run:

```bash
GOWORK=off go test ./...
```

Expected: PASS from the root module alone.

- [ ] **Step 5: Run import-boundary verification**

Run:

```bash
rg -n --glob '!examples/phase*' 'lockkit/(drivers/redis|idempotency/redis|guard/postgres)' README.md docs advanced backend guard idempotency redis examples
rg -n 'replace ' go.mod
```

Expected: no old adapter-path imports in supported root and adapter surfaces outside intentionally archived `examples/phase*`, and no root-module `replace` directives.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum go.work redis/go.mod redis/go.sum idempotency/redis/go.mod idempotency/redis/go.sum guard/postgres/go.mod guard/postgres/go.sum
git commit -m "refactor: finalize adapter module extraction"
```
