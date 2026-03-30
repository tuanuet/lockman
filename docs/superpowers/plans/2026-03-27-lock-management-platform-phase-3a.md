# Lock Management Platform Phase 3a Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add preview-quality strict execution and fencing-token support for single-resource runtime and worker flows without introducing guarded-write helpers or strict lineage/composite behavior.

**Architecture:** Extend the driver layer with a strict capability that issues fencing tokens, then thread that capability through registry detection, manager construction, runtime execution, worker execution, and the first strict-capable drivers. Keep strict support isolated to single-resource non-lineage flows so Phase 3a stays compatible with the existing standard-mode and Phase 2a lineage architecture. Documentation and examples must state clearly that Phase 3a exposes fencing tokens but does not complete the strict persistence safety story.

**Tech Stack:** Go, standard library `testing`, in-memory `testkit` driver, Redis driver and Lua scripts, existing `runtime`, `workers`, `registry`, and example binaries

---

### Task 1: Add Strict Driver Contracts And Manager Gating

**Files:**
- Modify: `lockkit/drivers/contracts.go`
- Modify: `lockkit/registry/registry.go`
- Modify: `lockkit/runtime/manager.go`
- Modify: `lockkit/workers/manager.go`
- Modify: `lockkit/registry/registry_test.go`
- Modify: `lockkit/runtime/exclusive_test.go`
- Modify: `lockkit/workers/manager_test.go`
- Modify: `lockkit/drivers/contracts_phase2_test.go`

- [ ] **Step 1: Write the failing contract and gating tests**

Add tests that lock the Phase 3a contract before production code changes:

```go
func TestRequiresStrictRuntimeDriverIgnoresAsyncOnlyStrictDefinitions(t *testing.T) {
	reg := registry.New()
	_ = reg.Register(definitions.LockDefinition{
		ID:                   "StrictAsyncOnly",
		Kind:                 definitions.KindParent,
		Resource:             "order",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionAsync,
		LeaseTTL:             time.Second,
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		IdempotencyRequired:  true,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	})

	if registry.RequiresStrictRuntimeDriver(reg) {
		t.Fatal("runtime strict gate should ignore async-only strict definitions")
	}
	if !registry.RequiresStrictWorkerDriver(reg) {
		t.Fatal("worker strict gate should include async-only strict definitions")
	}
}
```

Also add constructor tests proving:

- `runtime.NewManager(...)` rejects a registry with strict `sync` or `both` definitions when the driver lacks strict capability
- `workers.NewManager(...)` rejects a registry with strict `async` or `both` definitions when the driver lacks strict capability
- irrelevant strict definitions do not block manager construction
- registry validation rejects strict child definitions with non-empty `ParentRef`
- registry validation rejects standard child definitions whose parent definition is strict

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `go test ./lockkit/registry ./lockkit/runtime ./lockkit/workers ./lockkit/drivers -run 'Strict|RequiresStrict' -v`
Expected: FAIL overall because the strict driver contract, `RequiresStrictRuntimeDriver`, `RequiresStrictWorkerDriver`, and strict-capability manager gates do not exist yet. The new strict-lineage registry tests may already pass because that validation is already enforced by the existing Phase 2a lineage rules.

- [ ] **Step 3: Add the strict driver contract**

Extend `lockkit/drivers/contracts.go` with:

```go
type FencedLeaseRecord struct {
	Lease        LeaseRecord
	FencingToken uint64
}

type StrictAcquireRequest struct {
	DefinitionID string
	ResourceKey  string
	OwnerID      string
	LeaseTTL     time.Duration
}

type StrictDriver interface {
	AcquireStrict(ctx context.Context, req StrictAcquireRequest) (FencedLeaseRecord, error)
	RenewStrict(ctx context.Context, lease LeaseRecord, fencingToken uint64) (FencedLeaseRecord, error)
	ReleaseStrict(ctx context.Context, lease LeaseRecord, fencingToken uint64) error
}
```

Do not change the existing `Driver` or `LineageDriver` interfaces.

- [ ] **Step 4: Add registry detection helpers**

Add `RequiresStrictRuntimeDriver(reg Reader) bool` and `RequiresStrictWorkerDriver(reg Reader) bool` to `lockkit/registry/registry.go`.

Use the existing `Definitions()` snapshot and filter by:

- runtime: `ModeStrict` + `ExecutionSync` or `ExecutionBoth`
- workers: `ModeStrict` + `ExecutionAsync` or `ExecutionBoth`

Do not widen the existing `RequiresLineageDriver(...)` behavior.

- [ ] **Step 5: Keep registry validation logic unchanged and rely on regression tests**

Do not add new production validation logic for strict-lineage permutations in Phase 3a planning. The existing Phase 2a lineage validator already rejects:

- strict child definitions with non-empty `ParentRef`
- standard children that reference strict parents

Task 1 should anchor that existing behavior with explicit regression tests rather than duplicating validation logic.

- [ ] **Step 6: Gate manager construction on the new helpers**

Update:

- `lockkit/runtime/manager.go`
- `lockkit/workers/manager.go`

Required behavior:

- runtime manager rejects missing `StrictDriver` only when `registry.RequiresStrictRuntimeDriver(reg)` is true
- worker manager rejects missing `StrictDriver` only when `registry.RequiresStrictWorkerDriver(reg)` is true
- lineage gating remains unchanged and still uses `RequiresLineageDriver(reg)`

Use the same fail-fast style already used for lineage capability checks.

- [ ] **Step 7: Run the focused tests to verify they pass**

Run: `go test ./lockkit/registry ./lockkit/runtime ./lockkit/workers ./lockkit/drivers -run 'Strict|RequiresStrict' -v`
Expected: PASS

- [ ] **Step 8: Commit the contract and gating batch**

```bash
git add lockkit/drivers/contracts.go lockkit/registry/registry.go lockkit/registry/registry_test.go lockkit/runtime/manager.go lockkit/workers/manager.go lockkit/runtime/exclusive_test.go lockkit/workers/manager_test.go lockkit/drivers/contracts_phase2_test.go
git commit -m "feat: add strict driver contract and manager gates"
```

### Task 2: Add Strict Support To The Memory Test Driver

**Files:**
- Modify: `lockkit/testkit/memory_driver.go`
- Modify: `lockkit/testkit/memory_driver_test.go`

- [ ] **Step 1: Write the failing strict-memory-driver tests**

Add tests covering:

```go
func TestMemoryDriverAcquireStrictIssuesIncreasingTokens(t *testing.T) {
	driver := testkit.NewMemoryDriver()
	ctx := context.Background()

	first, err := driver.AcquireStrict(ctx, drivers.StrictAcquireRequest{
		DefinitionID: "order.strict",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict first returned error: %v", err)
	}
	if err := driver.ReleaseStrict(ctx, first.Lease, first.FencingToken); err != nil {
		t.Fatalf("ReleaseStrict first returned error: %v", err)
	}

	second, err := driver.AcquireStrict(ctx, drivers.StrictAcquireRequest{
		DefinitionID: "order.strict",
		ResourceKey:  "order:123",
		OwnerID:      "worker-b",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict second returned error: %v", err)
	}
	if second.FencingToken <= first.FencingToken {
		t.Fatalf("expected monotonic fencing tokens, first=%d second=%d", first.FencingToken, second.FencingToken)
	}
}
```

Also add tests proving:

- `RenewStrict(...)` preserves the same token
- `ReleaseStrict(...)` rejects wrong token
- strict counters are keyed by `(definitionID, resourceKey)` rather than resource key alone
- `AcquireStrict(...)` returns `drivers.ErrInvalidRequest` when `ResourceKey` is empty

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `go test ./lockkit/testkit -run 'Strict' -v`
Expected: FAIL because the memory driver does not implement `StrictDriver`

- [ ] **Step 3: Extend the memory driver state for strict fences**

Modify `lockkit/testkit/memory_driver.go` to add:

- a per-boundary strict counter map, for example `map[string]uint64`
- a strict lease state that remembers the active token for the held lease

Recommended boundary key:

```go
func strictBoundaryKey(definitionID, resourceKey string) string
```

The counter must survive release so the next acquire on the same strict boundary gets a larger token.

- [ ] **Step 4: Implement `AcquireStrict`, `RenewStrict`, and `ReleaseStrict`**

Rules:

- `AcquireStrict(...)` returns `drivers.ErrInvalidRequest` when `ResourceKey` is empty; otherwise it validates singular resource input, increments the counter atomically under the driver mutex, and returns `FencingToken > 0`
- `RenewStrict(...)` preserves the existing token, refreshes the lease timings, and returns a new `FencedLeaseRecord` containing the updated `LeaseRecord`
- `ReleaseStrict(...)` verifies both owner and token before deleting the active strict lease
- standard `Acquire/Renew/Release` behavior must remain unchanged

- [ ] **Step 5: Run the focused tests to verify they pass**

Run: `go test ./lockkit/testkit -run 'Strict' -v`
Expected: PASS

- [ ] **Step 6: Commit the memory-driver batch**

```bash
git add lockkit/testkit/memory_driver.go lockkit/testkit/memory_driver_test.go
git commit -m "feat: add strict fencing to memory driver"
```

### Task 3: Implement Strict Runtime Execution

**Files:**
- Modify: `lockkit/runtime/exclusive.go`
- Modify: `lockkit/runtime/exclusive_test.go`

- [ ] **Step 1: Write the failing strict-runtime tests**

Add tests covering:

```go
func TestExecuteExclusiveStrictPopulatesFencingToken(t *testing.T) {
	reg := registry.New()
	_ = reg.Register(definitions.LockDefinition{
		ID:                   "StrictOrderLock",
		Kind:                 definitions.KindParent,
		Resource:             "order",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionSync,
		LeaseTTL:             time.Second,
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	})

	mgr, err := runtime.NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
		DefinitionID: "StrictOrderLock",
		KeyInput:     map[string]string{"order_id": "123"},
		Ownership:    definitions.OwnershipMeta{OwnerID: "runtime-a"},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		if lease.FencingToken == 0 {
			t.Fatal("expected non-zero fencing token for strict runtime execution")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteExclusive returned error: %v", err)
	}
}
```

Also add tests for:

- standard mode still exposes `FencingToken == 0`
- repeated strict reacquire yields a larger token after release
- strict runtime does not start a renewal loop and still completes within one TTL window
- strict runtime reentrancy still returns `ErrReentrantAcquire`

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `go test ./lockkit/runtime -run 'Strict' -v`
Expected: FAIL because `ExecuteExclusive` does not populate fencing tokens yet

- [ ] **Step 3: Extend the runtime internal lease state**

Update `heldLease` in `lockkit/runtime/exclusive.go`:

```go
type heldLease struct {
	lease        drivers.LeaseRecord
	lineage      *drivers.LineageLeaseMeta
	fencingToken uint64
}
```

Use `0` to represent standard mode.

- [ ] **Step 4: Add strict acquire and release branching**

Update the runtime acquire/release path so:

- standard + no lineage still uses `Driver.Acquire`
- standard + lineage still uses `LineageDriver.AcquireWithLineage`
- strict + no lineage uses `StrictDriver.AcquireStrict`
- strict + lineage remains unsupported and returns `ErrPolicyViolation` if it somehow escapes registry validation

Populate `LeaseContext.FencingToken` only on the strict branch.

Reentrancy behavior stays unchanged in this phase:

- same-process reentrant acquire is still rejected by the existing guard path
- no nested strict callback path is introduced, so there is no inner lease-context token propagation rule to add in Phase 3a

Also update runtime release handling so:

- standard + no lineage still uses `Driver.Release`
- standard + lineage still uses `LineageDriver.ReleaseWithLineage`
- strict + no lineage uses `StrictDriver.ReleaseStrict`

- [ ] **Step 5: Keep runtime renewal out of scope**

Do not add a goroutine-based renewal loop to `ExecuteExclusive`.

The implementation should reflect the Phase 3a rule explicitly:

- `StrictDriver.RenewStrict` exists but is not called by runtime in this phase
- strict runtime execution remains a single-TTL critical section

- [ ] **Step 6: Run the focused tests to verify they pass**

Run: `go test ./lockkit/runtime -run 'Strict' -v`
Expected: PASS

- [ ] **Step 7: Commit the strict-runtime batch**

```bash
git add lockkit/runtime/exclusive.go lockkit/runtime/exclusive_test.go
git commit -m "feat: add strict runtime execution"
```

### Task 4: Implement Strict Worker Execution And Renewal

**Files:**
- Modify: `lockkit/workers/execute.go`
- Modify: `lockkit/workers/renewal.go`
- Modify: `lockkit/workers/execute_test.go`

- [ ] **Step 1: Write the failing strict-worker tests**

Add tests covering:

```go
func TestExecuteClaimedStrictPopulatesFencingToken(t *testing.T) {
	reg := registry.New()
	_ = reg.Register(definitions.LockDefinition{
		ID:                   "StrictWorkerLock",
		Kind:                 definitions.KindParent,
		Resource:             "order",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionAsync,
		LeaseTTL:             1500 * time.Millisecond,
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		IdempotencyRequired:  true,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	})

	mgr, err := workers.NewManager(reg, testkit.NewMemoryDriver(), idempotency.NewMemoryStore())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteClaimed(context.Background(), definitions.MessageClaimRequest{
		DefinitionID:   "StrictWorkerLock",
		KeyInput:       map[string]string{"order_id": "123"},
		IdempotencyKey: "msg:123",
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "worker-a",
			MessageID:     "message-123",
			Attempt:       1,
			ConsumerGroup: "cg",
		},
	}, func(ctx context.Context, claim definitions.ClaimContext) error {
		if claim.FencingToken == 0 {
			t.Fatal("expected non-zero fencing token")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteClaimed returned error: %v", err)
	}
}
```

Also add tests for:

- worker renewal preserves the same token across `RenewStrict(...)`
- standard worker claims still expose `FencingToken == 0`
- strict acquire-time driver errors still return directly from `ExecuteClaimed(...)` via the existing acquire-error path

- [ ] **Step 2: Run the focused tests to verify they fail**

Run: `go test ./lockkit/workers -run 'Strict' -v`
Expected: FAIL because the worker path does not populate or renew fencing tokens yet

- [ ] **Step 3: Extend worker renewal state**

Update `renewableLease` in `lockkit/workers/execute.go`:

```go
type renewableLease struct {
	lease        drivers.LeaseRecord
	lineage      *drivers.LineageLeaseMeta
	fencingToken uint64
}
```

- [ ] **Step 4: Add strict acquire branching in `ExecuteClaimed`**

For single-resource non-lineage claims:

- standard mode keeps using `Driver.Acquire`
- strict mode uses `StrictDriver.AcquireStrict`
- before the strict acquire, verify `acquirePlan.lineage == nil`
- strict + lineage remains unsupported and returns `ErrPolicyViolation` immediately if it somehow escapes registry validation

Populate `ClaimContext.FencingToken` from the strict path and preserve existing idempotency semantics.

- [ ] **Step 5: Add strict release branching in `releaseClaimLease`**

Update worker release handling so:

- standard + no lineage still uses `Driver.Release`
- standard + lineage still uses `LineageDriver.ReleaseWithLineage`
- strict + no lineage uses `StrictDriver.ReleaseStrict`

- [ ] **Step 6: Teach worker renewal to use `RenewStrict`**

Update `lockkit/workers/renewal.go` so:

- standard leases still use `Driver.Renew`
- lineage leases still use `LineageDriver.RenewWithLineage`
- strict non-lineage leases use `StrictDriver.RenewStrict`

The renewed lease must preserve the same fencing token value.

When `RenewStrict(...)` succeeds, the renewal loop must:

- assert that `FencedLeaseRecord.FencingToken` matches the currently held token
- treat a token mismatch as lease loss and surface it through the existing `ErrLeaseLost` path
- copy the refreshed `LeaseRecord` timings from `FencedLeaseRecord.Lease` back into the internal `renewableLease` state while preserving `renewableLease.fencingToken`

- [ ] **Step 7: Run the focused tests to verify they pass**

Run: `go test ./lockkit/workers -run 'Strict' -v`
Expected: PASS

- [ ] **Step 8: Commit the strict-worker batch**

```bash
git add lockkit/workers/execute.go lockkit/workers/renewal.go lockkit/workers/execute_test.go
git commit -m "feat: add strict worker execution"
```

### Task 5: Implement Strict Fencing In The Redis Driver

**Files:**
- Modify: `lockkit/drivers/redis/driver.go`
- Modify: `lockkit/drivers/redis/scripts.go`
- Modify: `lockkit/drivers/redis/driver_integration_test.go`

- [ ] **Step 1: Write the failing Redis strict-driver integration tests**

Add integration tests for:

- `AcquireStrict(...)` issues `FencingToken > 0`
- reacquire after release yields a larger token
- `RenewStrict(...)` preserves the same token
- `ReleaseStrict(...)` rejects wrong owner
- `ReleaseStrict(...)` rejects wrong token
- `AcquireStrict(...)` returns `drivers.ErrInvalidRequest` when `ResourceKey` is empty

Use a deterministic key boundary like:

```go
req := drivers.StrictAcquireRequest{
	DefinitionID: "order.strict",
	ResourceKey:  "order:123",
	OwnerID:      "worker-a",
	LeaseTTL:     2 * time.Second,
}
```

- [ ] **Step 2: Run the focused integration tests to verify they fail**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./lockkit/drivers/redis -run 'Strict' -v`
Expected: FAIL because the Redis driver does not implement strict fencing yet

- [ ] **Step 3: Add strict Redis key helpers and scripts**

Extend `lockkit/drivers/redis/driver.go` and `scripts.go` with:

- a strict fencing counter key namespace such as `lockman:lease:fence:{definition}:{resource}`
- a strict lease metadata namespace or encoded lease payload that stores owner + token atomically
- Lua-backed acquire, renew, and release scripts for strict leases

The strict acquire script must make lease acquisition and token issuance one atomic decision.

- [ ] **Step 4: Implement `AcquireStrict`, `RenewStrict`, and `ReleaseStrict`**

Required behavior:

- `AcquireStrict(...)` returns `drivers.ErrInvalidRequest` when `ResourceKey` is empty; otherwise it allocates a new token only when the lease is acquired
- `RenewStrict(...)` verifies owner and token, preserves token, refreshes TTL, and returns a `FencedLeaseRecord` containing the refreshed `LeaseRecord`
- `ReleaseStrict(...)` verifies owner and token before deleting the strict lease
- strict fencing keys must not collide with existing lease keys or lineage keys

- [ ] **Step 5: Run the focused integration tests to verify they pass**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./lockkit/drivers/redis -run 'Strict' -v`
Expected: PASS

- [ ] **Step 6: Commit the Redis strict-driver batch**

```bash
git add lockkit/drivers/redis/driver.go lockkit/drivers/redis/scripts.go lockkit/drivers/redis/driver_integration_test.go
git commit -m "feat: add strict fencing to redis driver"
```

### Task 6: Update Docs And Add Strict Examples

**Files:**
- Modify: `README.md`
- Modify: `docs/lock-definition-reference.md`
- Modify: `docs/runtime-vs-workers.md`
- Create: `examples/strict-sync-fencing/main.go`
- Create: `examples/strict-sync-fencing/main_test.go`
- Create: `examples/strict-sync-fencing/README.md`
- Create: `examples/strict-async-fencing/main.go`
- Create: `examples/strict-async-fencing/main_test.go`
- Create: `examples/strict-async-fencing/README.md`

- [ ] **Step 1: Write the failing example tests**

Create output-contract tests for the new examples.

Strict runtime example should expect lines like:

```go
expected := []string{
	"strict runtime lock: order:123",
	"fencing token first: 1",
	"fencing token second: 2",
	"teaching point: strict runtime exposes fencing tokens but still relies on one ttl window in phase3a",
	"shutdown: ok",
}
```

Strict worker example should expect lines like:

```go
expected := []string{
	"strict worker claim: order:123",
	"fencing token: 1",
	"idempotency after ack: completed",
	"teaching point: strict worker exposes fencing tokens; guarded writes still arrive in phase3b",
	"shutdown: ok",
}
```

- [ ] **Step 2: Run the example tests to verify they fail**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./examples/strict-sync-fencing ./examples/strict-async-fencing -v`
Expected: FAIL because the example files do not exist yet

- [ ] **Step 3: Implement the runtime strict example**

Create `examples/strict-sync-fencing/main.go` using the memory driver:

- register one strict sync definition
- execute it twice sequentially
- print the fencing token from both runs
- show the second token is larger than the first
- explicitly state that runtime strict execution still depends on one TTL window in Phase 3a

- [ ] **Step 4: Implement the worker strict example**

Create `examples/strict-async-fencing/main.go` using Redis:

- register one strict async definition
- execute one claimed worker callback
- print the fencing token and idempotency terminal state
- state clearly that persistence guarded writes are still Phase 3b work

- [ ] **Step 5: Update README and the lock-definition reference**

Required doc updates:

- `README.md`: add a `Phase 3a Status` section and link both new examples
- `docs/lock-definition-reference.md`: change the `Mode` guidance so it no longer says only Phase 2 standard behavior exists; document that Phase 3a adds preview-quality strict execution with fencing tokens and that guarded writes remain out of scope
- `docs/runtime-vs-workers.md`: add one short clarification that strict execution does not change package selection rules, and reference the strict examples where helpful

- [ ] **Step 6: Run the docs/example tests to verify they pass**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./examples/strict-sync-fencing ./examples/strict-async-fencing -v`
Expected: PASS

- [ ] **Step 7: Commit the docs and example batch**

```bash
git add README.md docs/lock-definition-reference.md docs/runtime-vs-workers.md examples/strict-sync-fencing examples/strict-async-fencing
git commit -m "docs: add strict mode examples and references"
```

### Task 7: Full Verification And Final Touch-Ups

**Files:**
- Verify: `lockkit/drivers/contracts.go`
- Verify: `lockkit/registry/registry.go`
- Verify: `lockkit/runtime/manager.go`
- Verify: `lockkit/runtime/exclusive.go`
- Verify: `lockkit/workers/manager.go`
- Verify: `lockkit/workers/execute.go`
- Verify: `lockkit/workers/renewal.go`
- Verify: `lockkit/testkit/memory_driver.go`
- Verify: `lockkit/drivers/redis/driver.go`
- Verify: `lockkit/drivers/redis/scripts.go`
- Verify: `README.md`
- Verify: `docs/lock-definition-reference.md`
- Verify: `docs/runtime-vs-workers.md`
- Verify: `examples/strict-sync-fencing/*`
- Verify: `examples/strict-async-fencing/*`

- [ ] **Step 1: Verify strict-focused packages**

Run: `go test ./lockkit/runtime ./lockkit/workers ./lockkit/testkit ./lockkit/registry ./lockkit/drivers -v`
Expected: PASS

- [ ] **Step 2: Verify Redis strict driver and strict examples**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./lockkit/drivers/redis ./examples/strict-async-fencing -v`
Expected: PASS

- [ ] **Step 3: Verify all examples**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./examples/... -v`
Expected: PASS

- [ ] **Step 4: Run the full repository test suite**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./...`
Expected: PASS

- [ ] **Step 5: Final commit only if touch-ups were needed during verification**

```bash
git add README.md docs/lock-definition-reference.md docs/runtime-vs-workers.md lockkit/drivers/contracts.go lockkit/registry/registry.go lockkit/runtime/manager.go lockkit/runtime/exclusive.go lockkit/workers/manager.go lockkit/workers/execute.go lockkit/workers/renewal.go lockkit/testkit/memory_driver.go lockkit/drivers/redis/driver.go lockkit/drivers/redis/scripts.go examples/strict-sync-fencing examples/strict-async-fencing
git commit -m "test: polish phase 3a strict mode rollout"
```
