# Lock Management Platform Phase 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build Phase 2 of the lock management platform SDK for Go with worker claim execution, Redis as the first production backend, idempotency contracts, child/composite standard-mode support, and hardened registry validation.

**Architecture:** Phase 2 keeps the public API aligned with the base spec while filling in the missing execution paths behind it. The implementation should stay registry-first and interface-driven: definitions and validation land first, then shared policy/runtime helpers, then Redis and idempotency backends, then worker orchestration that composes those pieces without embedding backend-specific logic.

**Tech Stack:** Go 1.22+, standard library, `testing` package, Redis client for Go, real-Redis integration tests via controlled local/container harness

---

## Planned File Structure

### Existing files to extend

- `go.mod`: add Redis dependencies needed for the production driver and Redis idempotency store
- `README.md`: document Phase 2 capabilities and Redis-backed verification commands
- `lockkit/definitions/types.go`: add overlap/composite enums and definition shapes
- `lockkit/definitions/ownership.go`: add async request/context shapes for worker claims and composite claims
- `lockkit/errors/errors.go`: add typed worker/idempotency/composite policy errors without breaking existing Phase 1 callers
- `lockkit/drivers/contracts.go`: keep the existing backend-agnostic contract, but ensure presence metadata remains sufficient for owner/expiry inspection
- `lockkit/registry/registry.go`: register and read composite definitions in addition to lock definitions
- `lockkit/registry/validation.go`: validate child/composite invariants and overlap-policy rules
- `lockkit/testkit/memory_driver.go`: support Phase 2 presence/composite test behavior where useful without pretending to be a production backend
- `lockkit/testkit/assertions.go`: add helpers for composite/resource-set assertions if current tests become repetitive
- `lockkit/runtime/manager.go`: keep lifecycle management shared by sync and worker paths

### New Phase 2 packages and files

- `lockkit/internal/policy/composite.go`: canonical ordering helpers and member normalization
- `lockkit/internal/policy/overlap.go`: parent-child overlap rejection rules
- `lockkit/internal/policy/outcome.go`: `WorkerOutcome` mapping and `OutcomeFromError`
- `lockkit/runtime/composite.go`: standard composite sync execution
- `lockkit/runtime/composite_test.go`: composite sync lifecycle tests
- `lockkit/idempotency/contracts.go`: `Store`, record model, begin/complete/fail inputs
- `lockkit/idempotency/memory_store.go`: in-memory idempotency store for worker unit tests
- `lockkit/idempotency/memory_store_test.go`: contract tests for the in-memory store
- `lockkit/idempotency/redis/store.go`: Redis-backed idempotency store
- `lockkit/idempotency/redis/store_integration_test.go`: real-Redis integration tests for idempotency transitions
- `lockkit/drivers/redis/driver.go`: Redis production lock driver
- `lockkit/drivers/redis/scripts.go`: Lua scripts or script loaders for owner-checked renew/release
- `lockkit/drivers/redis/driver_integration_test.go`: real-Redis driver tests
- `lockkit/workers/manager.go`: worker manager construction and dependencies
- `lockkit/workers/execute.go`: single-resource `ExecuteClaimed`
- `lockkit/workers/execute_composite.go`: `ExecuteCompositeClaimed`
- `lockkit/workers/shutdown.go`: worker shutdown admission/drain logic
- `lockkit/workers/renewal.go`: SDK-owned renewal loop for worker claims
- `lockkit/workers/manager_test.go`: constructor and shutdown tests
- `lockkit/workers/execute_test.go`: single-resource worker tests
- `lockkit/workers/execute_composite_test.go`: composite worker tests

### Existing tests to extend

- `lockkit/registry/registry_test.go`
- `lockkit/runtime/shutdown_test.go`

## Phase Scope

This plan delivers only what the Phase 2 spec/design requires:

- worker claim execution through a queue-agnostic API
- Redis as the first production driver
- idempotency through an interface boundary
- child-lock overlap rejection
- standard composite execution for sync and worker paths
- backend compatibility and validation hardening

It does **not** implement:

- strict runtime execution
- fencing tokens
- guarded persistence writes
- auto-escalation from child to parent
- queue-product-specific middleware
- strict composite runtime behavior

## Task 1: Extend Definition, Request, And Error Contracts

**Files:**
- Modify: `lockkit/definitions/types.go`
- Modify: `lockkit/definitions/ownership.go`
- Modify: `lockkit/errors/errors.go`
- Test: `lockkit/definitions/types_phase2_test.go`
- Test: `lockkit/definitions/ownership_phase2_test.go`
- Test: `lockkit/errors/errors_phase2_test.go`

- [ ] **Step 1: Write the failing Phase 2 contract tests**

```go
func TestCompositeDefinitionCarriesPhase2Shape(t *testing.T) {
	def := definitions.CompositeDefinition{
		ID:               "transfer",
		Members:          []string{"account.debit", "account.credit"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   2,
		ExecutionKind:    definitions.ExecutionBoth,
	}

	if len(def.Members) != 2 {
		t.Fatalf("expected composite members, got %d", len(def.Members))
	}
}

func TestLockDefinitionCarriesOverlapPolicy(t *testing.T) {
	def := definitions.LockDefinition{
		ID:            "order.item",
		Kind:          definitions.KindChild,
		OverlapPolicy: definitions.OverlapReject,
	}

	if def.OverlapPolicy != definitions.OverlapReject {
		t.Fatalf("expected overlap policy to be preserved, got %q", def.OverlapPolicy)
	}
}

func TestMessageClaimRequestCarriesIdempotencyKey(t *testing.T) {
	req := definitions.MessageClaimRequest{IdempotencyKey: "msg:123"}
	if req.IdempotencyKey == "" {
		t.Fatal("expected idempotency key to be preserved")
	}
}

func TestClaimContextSupportsCompositeResources(t *testing.T) {
	claim := definitions.ClaimContext{
		ResourceKeys:   []string{"account:a", "account:b"},
		IdempotencyKey: "msg:123",
	}

	if len(claim.ResourceKeys) != 2 {
		t.Fatalf("expected composite resource keys, got %d", len(claim.ResourceKeys))
	}
}
```

- [ ] **Step 2: Run the contract tests to verify they fail**

Run: `go test ./lockkit/definitions ./lockkit/errors -run 'Phase2|CompositeDefinition|MessageClaimRequest' -v`
Expected: FAIL with undefined `CompositeDefinition`, missing worker request/context types, or missing Phase 2 error values

- [ ] **Step 3: Add the minimum public shapes required by the Phase 2 design**

```go
type OverlapPolicy string

const (
	OverlapReject   OverlapPolicy = "reject"
	OverlapEscalate OverlapPolicy = "escalate"
)

type CompositeDefinition struct {
	ID               string
	Members          []string
	OrderingPolicy   OrderingPolicy
	AcquirePolicy    AcquirePolicy
	EscalationPolicy EscalationPolicy
	ModeResolution   ModeResolution
	MaxMemberCount   int
	ExecutionKind    ExecutionKind
}

type LockDefinition struct {
	ID                   string
	Kind                 LockKind
	Resource             string
	Mode                 LockMode
	ExecutionKind        ExecutionKind
	LeaseTTL             time.Duration
	WaitTimeout          time.Duration
	RetryPolicy          RetryPolicy
	BackendFailurePolicy BackendFailurePolicy
	FencingRequired      bool
	IdempotencyRequired  bool
	CheckOnlyAllowed     bool
	Rank                 int
	ParentRef            string
	OverlapPolicy        OverlapPolicy
	KeyBuilder           KeyBuilder
	Tags                 map[string]string
}

type MessageClaimRequest struct {
	DefinitionID   string
	KeyInput       map[string]string
	Ownership      OwnershipMeta
	IdempotencyKey string
	Overrides      *RuntimeOverrides
}

type CompositeLockRequest struct {
	DefinitionID string
	MemberInputs []map[string]string
	Ownership    OwnershipMeta
	Overrides    *RuntimeOverrides
}

type CompositeClaimRequest struct {
	DefinitionID   string
	MemberInputs   []map[string]string
	Ownership      OwnershipMeta
	IdempotencyKey string
	Overrides      *RuntimeOverrides
}

type LeaseContext struct {
	DefinitionID  string
	ResourceKey   string
	ResourceKeys  []string
	Ownership     OwnershipMeta
	FencingToken  uint64
	LeaseTTL      time.Duration
	LeaseDeadline time.Time
}

type ClaimContext struct {
	DefinitionID   string
	ResourceKey    string
	ResourceKeys   []string
	Ownership      OwnershipMeta
	FencingToken   uint64
	LeaseTTL       time.Duration
	LeaseDeadline  time.Time
	IdempotencyKey string
}
```

- [ ] **Step 4: Add typed error values needed by composite and worker orchestration**

```go
var (
	ErrDuplicateIgnored = stdErrors.New("duplicate ignored")
	ErrInvariantRejected = stdErrors.New("invariant rejected")
	ErrWorkerShuttingDown = stdErrors.New("worker shutting down")
)
```

- [ ] **Step 5: Run package tests to verify the public contract compiles**

Run: `go test ./lockkit/definitions ./lockkit/errors -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add lockkit/definitions/types.go lockkit/definitions/ownership.go lockkit/errors/errors.go lockkit/definitions/types_phase2_test.go lockkit/definitions/ownership_phase2_test.go lockkit/errors/errors_phase2_test.go
git commit -m "feat(definitions): add phase 2 worker and composite contracts"
```

## Task 2: Extend Registry Storage And Validation For Child/Composite Definitions

**Files:**
- Modify: `lockkit/registry/registry.go`
- Modify: `lockkit/registry/validation.go`
- Modify: `lockkit/registry/registry_test.go`

- [ ] **Step 1: Write failing registry tests for child/composite registration**

```go
func TestRegistryRegisterCompositeStoresDefinition(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(validParentDefinition()); err != nil {
		t.Fatalf("Register returned error: %v", err)
	}

	err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:               "transfer",
		Members:          []string{"account.parent"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   1,
		ExecutionKind:    definitions.ExecutionBoth,
	})
	if err != nil {
		t.Fatalf("RegisterComposite returned error: %v", err)
	}

	got := reg.MustGetComposite("transfer")
	if got.ID != "transfer" {
		t.Fatalf("expected composite definition, got %#v", got)
	}
}
```

- [ ] **Step 2: Run registry tests to verify they fail**

Run: `go test ./lockkit/registry -run 'Composite|ParentRef|Overlap' -v`
Expected: FAIL with undefined composite registration/lookup or missing validation failures

- [ ] **Step 3: Extend the registry read/write surface**

```go
type Reader interface {
	MustGet(id string) definitions.LockDefinition
	MustGetComposite(id string) definitions.CompositeDefinition
}

type Registry struct {
	definitions map[string]definitions.LockDefinition
	composites  map[string]definitions.CompositeDefinition
}
```

Any custom `registry.Reader` test doubles that currently satisfy the Phase 1 interface must be updated in the same task to implement `MustGetComposite`, otherwise downstream runtime tests will stop compiling.

- [ ] **Step 4: Add validation rules required by the Phase 2 design**

Validation must reject:
- child definitions with unknown `ParentRef`
- unsupported child `OverlapPolicy`
- composite definitions with unknown members
- composite definitions containing any `strict` member
- mixed-mode composite members
- composite definitions whose policy fields differ from the Phase 2-supported constants
- composite definitions that exceed `MaxMemberCount`
- composite IDs that collide with lock definition IDs
- strict async or strict both definitions without `IdempotencyRequired`

- [ ] **Step 5: Run registry tests**

Run: `go test ./lockkit/registry -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add lockkit/registry/registry.go lockkit/registry/validation.go lockkit/registry/registry_test.go
git commit -m "feat(registry): validate child and composite definitions"
```

## Task 3: Add Shared Policy Helpers And Sync Composite Runtime

**Files:**
- Create: `lockkit/internal/policy/composite.go`
- Create: `lockkit/internal/policy/overlap.go`
- Create: `lockkit/runtime/composite.go`
- Create: `lockkit/runtime/composite_test.go`
- Modify: `lockkit/runtime/manager.go`
- Modify: `lockkit/runtime/shutdown_test.go`

- [ ] **Step 1: Write failing composite runtime tests**

```go
func TestExecuteCompositeExclusiveAcquiresMembersInCanonicalOrder(t *testing.T) {
	reg := newCompositeRegistry(t)
	driver := testkit.NewMemoryDriver()
	mgr, err := runtime.NewManager(reg, driver, nil)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	var visited []string
	err = mgr.ExecuteCompositeExclusive(context.Background(), compositeRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
		visited = append(visited, lease.ResourceKeys...)
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteCompositeExclusive returned error: %v", err)
	}
	if len(visited) != 2 {
		t.Fatalf("expected 2 composite members, got %d", len(visited))
	}
}
```

- [ ] **Step 2: Run runtime tests to verify they fail**

Run: `go test ./lockkit/runtime -run 'Composite|Overlap' -v`
Expected: FAIL with missing `ExecuteCompositeExclusive` or overlap-policy enforcement

- [ ] **Step 3: Implement canonical ordering and overlap helpers in `internal/policy`**

```go
func CanonicalizeMembers(defs []definitions.LockDefinition, keys []string) ([]MemberLeasePlan, error)
func RejectOverlap(parent definitions.LockDefinition, child definitions.LockDefinition, parentKey string, childKey string) error
```

Canonical ordering must follow the base spec exactly:

1. lower `Rank` first
2. then lexicographic `Resource`
3. then lexicographic normalized resource key

- [ ] **Step 4: Implement standard-mode composite sync execution**

Required behavior:
- resolve composite definition and member definitions
- build member keys
- sort members canonically
- acquire member leases sequentially
- release acquired members in reverse order on failure
- reject parent-child overlap when the normalized tree collides

For Phase 2, treat the normalized resource key as the final built key string. Overlap rejection should at minimum reject:

- exact key equality between parent and child
- child keys prefixed by `parentKey + ":"`

- [ ] **Step 5: Run runtime tests**

Run: `go test ./lockkit/runtime -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add lockkit/internal/policy/composite.go lockkit/internal/policy/overlap.go lockkit/runtime/composite.go lockkit/runtime/composite_test.go lockkit/runtime/manager.go lockkit/runtime/shutdown_test.go
git commit -m "feat(runtime): add standard composite sync execution"
```

## Task 4: Add Idempotency Contracts And In-Memory Test Store

**Files:**
- Create: `lockkit/idempotency/contracts.go`
- Create: `lockkit/idempotency/memory_store.go`
- Create: `lockkit/idempotency/memory_store_test.go`

- [ ] **Step 1: Write failing idempotency-store tests**

```go
func TestMemoryStoreBeginRejectsSecondActiveClaim(t *testing.T) {
	store := idempotency.NewMemoryStore()

	first, err := store.Begin(context.Background(), "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       1,
		TTL:           time.Minute,
	})
	if err != nil {
		t.Fatalf("first Begin returned error: %v", err)
	}
	if !first.Acquired {
		t.Fatal("expected first Begin to acquire slot")
	}

	second, err := store.Begin(context.Background(), "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-b",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       2,
		TTL:           time.Minute,
	})
	if err != nil {
		t.Fatalf("second Begin returned error: %v", err)
	}
	if second.Acquired {
		t.Fatal("expected duplicate Begin to be rejected")
	}
}
```

- [ ] **Step 2: Run idempotency tests to verify they fail**

Run: `go test ./lockkit/idempotency -v`
Expected: FAIL with missing package/files

- [ ] **Step 3: Implement the contract and in-memory store**

```go
type Status string

const (
	StatusMissing    Status = "missing"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
)

type Record struct {
	Key           string
	Status        Status
	OwnerID       string
	MessageID     string
	ConsumerGroup string
	Attempt       int
	UpdatedAt     time.Time
	ExpiresAt     time.Time
}

type BeginInput struct {
	OwnerID       string
	MessageID     string
	ConsumerGroup string
	Attempt       int
	TTL           time.Duration
}

type BeginResult struct {
	Record    Record
	Acquired  bool
	Duplicate bool
}

type CompleteInput struct {
	OwnerID   string
	MessageID string
	TTL       time.Duration
}

type FailInput struct {
	OwnerID   string
	MessageID string
	TTL       time.Duration
}

type Store interface {
	Get(ctx context.Context, key string) (Record, error)
	Begin(ctx context.Context, key string, input BeginInput) (BeginResult, error)
	Complete(ctx context.Context, key string, input CompleteInput) error
	Fail(ctx context.Context, key string, input FailInput) error
}
```

Behavior rules:
- `BeginInput.TTL` is in-progress TTL
- `CompleteInput.TTL` and `FailInput.TTL` are terminal retention TTLs
- caller owns idempotency-key generation; empty required keys are rejected by worker runtime, not silently derived here
- `BeginResult.Acquired` distinguishes a newly claimed processing slot from a duplicate
- `BeginResult.Duplicate` distinguishes duplicate/replayed work from a fresh claim
- `Record` must preserve owner/message metadata needed by later retries and diagnostics

- [ ] **Step 4: Run idempotency tests**

Run: `go test ./lockkit/idempotency -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lockkit/idempotency/contracts.go lockkit/idempotency/memory_store.go lockkit/idempotency/memory_store_test.go
git commit -m "feat(idempotency): add contracts and memory store"
```

## Task 5: Implement Redis Production Lock Driver

**Files:**
- Modify: `go.mod`
- Modify: `lockkit/drivers/contracts.go`
- Create: `lockkit/drivers/redis/driver.go`
- Create: `lockkit/drivers/redis/scripts.go`
- Create: `lockkit/drivers/redis/driver_integration_test.go`
- Test: `lockkit/drivers/contracts_phase2_test.go`

- [ ] **Step 1: Define the Redis integration-test harness before writing driver code**

Use one discovery path for all Redis-backed Phase 2 tests:

- integration tests read `LOCKMAN_REDIS_URL`
- when unset, integration tests call `t.Skip("LOCKMAN_REDIS_URL is not set")`
- CI or local harness provides a real Redis instance through that env var

- [ ] **Step 2: Write failing Redis driver integration tests**

```go
func TestDriverReleaseRejectsWrongOwner(t *testing.T) {
	driver := newRedisDriverForTest(t)
	ctx := context.Background()

	lease, err := driver.Acquire(ctx, drivers.AcquireRequest{
		DefinitionID: "order.lock",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "worker-a",
		LeaseTTL:     time.Minute,
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	err = driver.Release(ctx, drivers.LeaseRecord{
		DefinitionID: lease.DefinitionID,
		ResourceKeys: lease.ResourceKeys,
		OwnerID:      "worker-b",
		LeaseTTL:     lease.LeaseTTL,
	})
	if !errors.Is(err, drivers.ErrLeaseOwnerMismatch) {
		t.Fatalf("expected owner mismatch, got %v", err)
	}
}

func TestDriverCheckPresenceReturnsOwnerAndExpiry(t *testing.T) {
	driver := newRedisDriverForTest(t)
	ctx := context.Background()

	lease, err := driver.Acquire(ctx, drivers.AcquireRequest{
		DefinitionID: "order.lock",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "worker-a",
		LeaseTTL:     time.Minute,
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	record, err := driver.CheckPresence(ctx, drivers.PresenceRequest{
		DefinitionID: "order.lock",
		ResourceKeys: []string{"order:123"},
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}
	if !record.Present || record.Lease.OwnerID != lease.OwnerID || record.Lease.ExpiresAt.IsZero() {
		t.Fatalf("expected owner and expiry metadata, got %#v", record)
	}
}
```

- [ ] **Step 3: Run the Redis driver tests to verify they fail**

Run: `go test ./lockkit/drivers ./lockkit/drivers/redis -v`
Expected: FAIL with missing package/files

- [ ] **Step 4: Add Redis dependencies and implement the driver**

Required behavior:
- `Acquire` uses atomic create-if-absent with TTL
- `Renew` is owner-checked
- `Release` is owner-checked
- `CheckPresence` returns owner and expiry metadata through `PresenceRecord`
- `Ping` provides startup compatibility verification
- `drivers/contracts.go` remains the compile-time contract and must not grow a Redis-only `Inspect` method

- [ ] **Step 5: Use Lua/script-backed paths for renew and release**

```go
var renewScript = redis.NewScript(`...`)
var releaseScript = redis.NewScript(`...`)
```

- [ ] **Step 6: Run Redis driver tests**

Run: `go test ./lockkit/drivers/redis -v`
Expected: PASS against the configured local/container Redis harness

- [ ] **Step 7: Commit**

```bash
git add go.mod lockkit/drivers/contracts.go lockkit/drivers/contracts_phase2_test.go lockkit/drivers/redis/driver.go lockkit/drivers/redis/scripts.go lockkit/drivers/redis/driver_integration_test.go
git commit -m "feat(drivers): add redis production driver"
```

## Task 6: Implement Redis Idempotency Store

**Files:**
- Create: `lockkit/idempotency/redis/store.go`
- Create: `lockkit/idempotency/redis/store_integration_test.go`

- [ ] **Step 1: Reuse the same Redis integration harness from Task 5**

All Redis idempotency tests must use `LOCKMAN_REDIS_URL` and skip clearly when it is unset.

- [ ] **Step 2: Write failing Redis idempotency integration tests**

```go
func TestStoreCompletePersistsTerminalRecord(t *testing.T) {
	store := newRedisStoreForTest(t)
	ctx := context.Background()

	_, err := store.Begin(ctx, "msg:123", idempotency.BeginInput{
		OwnerID:       "worker-a",
		MessageID:     "123",
		ConsumerGroup: "payments",
		Attempt:       1,
		TTL:           time.Minute,
	})
	if err != nil {
		t.Fatalf("Begin returned error: %v", err)
	}

	if err := store.Complete(ctx, "msg:123", idempotency.CompleteInput{
		OwnerID:   "worker-a",
		MessageID: "123",
		TTL:       24 * time.Hour,
	}); err != nil {
		t.Fatalf("Complete returned error: %v", err)
	}

	record, err := store.Get(ctx, "msg:123")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if record.Status != idempotency.StatusCompleted {
		t.Fatalf("expected completed record, got %q", record.Status)
	}
}
```

- [ ] **Step 3: Run Redis idempotency tests to verify they fail**

Run: `go test ./lockkit/idempotency/redis -v`
Expected: FAIL with missing package/files

- [ ] **Step 4: Implement Redis-backed `Begin`, `Get`, `Complete`, and `Fail`**

Implementation rules:
- `Begin` must be atomic
- terminal transitions must preserve observability metadata
- terminal retention TTL must differ from in-progress TTL
- Redis implementation stays behind the `idempotency.Store` interface

- [ ] **Step 5: Run Redis idempotency tests**

Run: `go test ./lockkit/idempotency/redis -v`
Expected: PASS against the configured local/container Redis harness

- [ ] **Step 6: Commit**

```bash
git add lockkit/idempotency/redis/store.go lockkit/idempotency/redis/store_integration_test.go
git commit -m "feat(idempotency): add redis store"
```

## Task 7: Implement Single-Resource Worker Claim Execution

**Files:**
- Create: `lockkit/workers/manager.go`
- Create: `lockkit/workers/execute.go`
- Create: `lockkit/workers/shutdown.go`
- Create: `lockkit/workers/renewal.go`
- Create: `lockkit/workers/manager_test.go`
- Create: `lockkit/workers/execute_test.go`
- Create: `lockkit/internal/policy/outcome.go`

- [ ] **Step 1: Write failing worker tests for single-resource execution**

```go
func TestExecuteClaimedPersistsIdempotencyBeforeAck(t *testing.T) {
	mgr := newWorkerManagerForTest(t)

	err := mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		if claim.IdempotencyKey == "" {
			t.Fatal("expected claim idempotency key")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteClaimed returned error: %v", err)
	}

	record, err := mgr.testStore.Get(context.Background(), "msg:123")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if record.Status != idempotency.StatusCompleted {
		t.Fatalf("expected completed status, got %q", record.Status)
	}
}

func TestExecuteClaimedCancelsContextWhenRenewalFails(t *testing.T) {
	mgr := newWorkerManagerWithRenewFailure(t)

	err := mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, lockerrors.ErrLeaseLost) {
		t.Fatalf("expected lease lost after renew failure, got %v", err)
	}
}
```

- [ ] **Step 2: Run worker tests to verify they fail**

Run: `go test ./lockkit/workers -run 'ExecuteClaimed|OutcomeFromError|Shutdown' -v`
Expected: FAIL with missing worker package/files

- [ ] **Step 3: Implement worker manager construction and admission/shutdown**

Required behavior:
- constructor validates registry and backend compatibility
- worker manager implements `Shutdown(ctx) error`
- new claims are rejected after shutdown starts
- in-flight worker executions are drained before shutdown returns or context expires

- [ ] **Step 4: Implement `ExecuteClaimed` and normalized outcome mapping**

Required behavior:
- validate definition, metadata, and required idempotency key
- consult `idempotency.Store` before acquire
- reject same-process reentrant claims on identical definition/resource-set
- start SDK-owned renewal after acquire
- run renewal on the Phase 2 baseline cadence of `LeaseTTL / 3`, subject to implementation-level min/max clamps
- on renewal failure, cancel the worker callback context immediately
- map renewal failure to `ErrLeaseLost`, then route that error through normal outcome mapping
- attempt best-effort release after renewal failure or callback cancellation
- persist terminal idempotency state before allowing `OutcomeFromError(nil)` to map to `ack`
- call `Complete` for successful `ack`
- call `Complete` for duplicate-terminal `ack`
- call `Fail` for `drop` and `dlq`
- leave the record `in_progress` for `retry` so the processing-slot TTL controls duplicate retry behavior

Idempotency decision table:

- `missing` -> continue to acquire and execute callback
- `in_progress` -> do not execute callback; return retry-mapped error; leave record as-is
- `completed` -> do not execute callback; return duplicate-terminal ack path; call `Complete` only if the runtime needs to refresh terminal retention
- `failed` -> do not execute callback by default; return retry-mapped error unless later policy says otherwise

- [ ] **Step 5: Add an explicit outcome-mapping table test**

```go
func TestOutcomeFromErrorMapsWorkerErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want policy.WorkerOutcome
	}{
		{name: "nil", err: nil, want: policy.OutcomeAck},
		{name: "busy", err: lockerrors.ErrLockBusy, want: policy.OutcomeRetry},
		{name: "duplicate ignored", err: lockerrors.ErrDuplicateIgnored, want: policy.OutcomeAck},
		{name: "policy violation", err: lockerrors.ErrPolicyViolation, want: policy.OutcomeDrop},
	}
	_ = cases
}
```

- [ ] **Step 6: Run worker tests**

Run: `go test ./lockkit/workers -run 'ExecuteClaimed|OutcomeFromError|Shutdown' -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add lockkit/workers/manager.go lockkit/workers/execute.go lockkit/workers/shutdown.go lockkit/workers/renewal.go lockkit/workers/manager_test.go lockkit/workers/execute_test.go lockkit/internal/policy/outcome.go
git commit -m "feat(workers): add single-resource claim execution"
```

## Task 8: Implement Composite Worker Execution And Full Phase 2 Verification

**Files:**
- Create: `lockkit/workers/execute_composite.go`
- Create: `lockkit/workers/execute_composite_test.go`
- Modify: `README.md`

- [ ] **Step 1: Write failing composite worker tests**

```go
func TestExecuteCompositeClaimedRollsBackOnPartialAcquireFailure(t *testing.T) {
	mgr := newCompositeWorkerManagerForTest(t)

	err := mgr.ExecuteCompositeClaimed(context.Background(), compositeClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected composite acquire failure")
	}

	testkit.AssertNotHeld(t, mgr.testDriver, "account:a")
	testkit.AssertNotHeld(t, mgr.testDriver, "account:b")
}
```

- [ ] **Step 2: Run composite worker tests to verify they fail**

Run: `go test ./lockkit/workers -run 'ExecuteCompositeClaimed|Reentrant|Rollback' -v`
Expected: FAIL with missing composite worker implementation

- [ ] **Step 3: Implement `ExecuteCompositeClaimed`**

Required behavior:
- resolve composite definition and member definitions
- build member keys from `MemberInputs`
- reject unsupported overlap before acquisition
- order members canonically
- acquire sequentially and roll back in reverse order on failure
- populate `ClaimContext.ResourceKeys`
- reuse single-resource outcome/idempotency/shutdown rules

- [ ] **Step 4: Run the full repository test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 5: Update `README.md` with Phase 2 status and Redis verification notes**

```md
## Phase 2 Status

- Worker claim execution via `ExecuteClaimed` and `ExecuteCompositeClaimed`
- Redis production driver and Redis-backed idempotency store
- Child overlap rejection and standard composite execution
```

- [ ] **Step 6: Run the full repository test suite again after docs/code touch-up**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add lockkit/workers/execute_composite.go lockkit/workers/execute_composite_test.go README.md
git commit -m "feat(workers): add composite claim execution"
```

## Suggested Execution Order

Recommended order:

1. Task 1
2. Task 2
3. Task 3
4. Task 4
5. Tasks 5 and 6 can run in parallel after Task 4 if isolated carefully
6. Task 7
7. Task 8

Rationale:

- Tasks 1-4 define the shared contracts the rest of Phase 2 depends on
- Redis driver and Redis idempotency store are mostly independent once contracts exist
- worker execution should start only after contracts, policy helpers, and backends are stable

## Verification Checklist

Before declaring Phase 2 implementation complete, verify all of the following:

- `go test ./lockkit/definitions ./lockkit/errors ./lockkit/registry -v`
- `go test ./lockkit/runtime -v`
- `go test ./lockkit/idempotency -v`
- `go test ./lockkit/drivers/redis -v`
- `go test ./lockkit/idempotency/redis -v`
- `go test ./lockkit/workers -v`
- `go test ./...`

If Redis integration tests require local setup, document the exact harness command in `README.md` during Task 8 rather than relying on tribal knowledge.
