# Lock Management Platform Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a production-shaped Phase 1 foundation of the lock management platform SDK for Go with central registry enforcement, standard-mode exclusive execution, presence checks, an in-memory driver, and baseline observability.

**Architecture:** Phase 1 intentionally stops at the standard-mode foundation so the library can compile, run tests, and prove the core boundaries before adding strict mode and async workers. The implementation centers on a registry-first API, deterministic key building, a non-reentrant runtime manager, and an in-memory driver that exercises the same driver contract future production backends will implement.

**Tech Stack:** Go 1.22+, standard library, OpenTelemetry API for metrics/tracing interfaces, `testing` package

---

## Planned File Structure

### Repository bootstrap

- `go.mod`: Go module definition for the library workspace
- `README.md`: short project overview and local development commands
- `.gitignore`: ignore Go build and test artifacts

### Core library packages

- `lockkit/definitions/types.go`: enums and shared definition structs
- `lockkit/definitions/key_builder.go`: `KeyBuilder` contract and template-backed implementation
- `lockkit/definitions/ownership.go`: ownership metadata and request/context structs
- `lockkit/errors/errors.go`: typed error values and helpers
- `lockkit/observe/contracts.go`: observability contracts and no-op defaults
- `lockkit/drivers/contracts.go`: driver interface, lease record, presence state contract
- `lockkit/registry/registry.go`: definition registration and lookup
- `lockkit/registry/validation.go`: registry validation rules
- `lockkit/testkit/memory_driver.go`: in-memory driver implementation
- `lockkit/testkit/assertions.go`: test helpers for lock state assertions
- `lockkit/runtime/manager.go`: standard-mode `LockManager`, `LockInspector`, lifecycle shutdown
- `lockkit/runtime/exclusive.go`: single-resource exclusive execution flow
- `lockkit/runtime/presence.go`: presence-check implementation
- `lockkit/runtime/metrics.go`: runtime instrumentation helpers

### Tests

- `lockkit/definitions/key_builder_test.go`
- `lockkit/errors/errors_test.go`
- `lockkit/registry/registry_test.go`
- `lockkit/testkit/memory_driver_test.go`
- `lockkit/runtime/exclusive_test.go`
- `lockkit/runtime/presence_test.go`
- `lockkit/runtime/shutdown_test.go`

### Deferred to later phases

- `lockkit/runtime/composite.go`: composite standard execution
- `lockkit/workers/`: async worker claim flow
- `lockkit/guard/`: strict-mode guarded writes
- `lockkit/integration/`: boundary decorators and middleware

## Phase Scope

This plan delivers only what Phase 1 in the spec requires:

- central registry
- parent-lock capable standard mode
- presence check
- in-memory driver and testing support
- baseline observability hooks

It does **not** implement:

- composite execution
- strict mode
- worker claim flow
- guarded write helpers
- production driver

## Task 1: Bootstrap Go Module And Workspace

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `README.md`

- [ ] **Step 1: Write the failing smoke test command**

Run: `go test ./...`
Expected: FAIL with `pattern ./...: directory prefix . does not contain main module`

- [ ] **Step 2: Initialize the module and basic workspace files**

```go
module github.com/tuanuet/lockman

go 1.22
```

```gitignore
.DS_Store
coverage.out
*.test
```

```md
# lockman

Distributed lock platform SDK prototype for Go.

## Commands

- `go test ./...`
- `go test ./... -cover`
```

- [ ] **Step 3: Verify the workspace can load as a Go module**

Run: `go test ./...`
Expected: the command resolves the Go module successfully; on an empty bootstrap workspace it may still report `matched no packages` / `no packages to test`

- [ ] **Step 4: Initialize git if the workspace is still not a repository**

Run: `git rev-parse --show-toplevel || git init`
Expected: either existing repository path or `Initialized empty Git repository`

- [ ] **Step 5: Commit**

```bash
git add go.mod .gitignore README.md
git commit -m "chore: bootstrap lockman workspace"
```

## Task 2: Add Core Definition Types And Key Builders

**Files:**
- Create: `lockkit/definitions/types.go`
- Create: `lockkit/definitions/key_builder.go`
- Create: `lockkit/definitions/ownership.go`
- Test: `lockkit/definitions/key_builder_test.go`

- [ ] **Step 1: Write the failing key-builder tests**

```go
func TestTemplateKeyBuilderBuildsDeterministicKey(t *testing.T) {
	builder := definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}")

	key, err := builder.Build(map[string]string{
		"order_id": "123",
		"item_id":  "9",
	})

	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if key != "order:123:item:9" {
		t.Fatalf("expected deterministic key, got %q", key)
	}
}

func TestTemplateKeyBuilderRejectsMissingField(t *testing.T) {
	builder := definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}")

	_, err := builder.Build(map[string]string{"order_id": "123"})
	if err == nil {
		t.Fatal("expected missing field error")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./lockkit/definitions -run KeyBuilder -v`
Expected: FAIL with `no Go files` or undefined symbols

- [ ] **Step 3: Write the minimal definitions and builder implementation**

```go
type LockMode string

const (
	ModeStandard LockMode = "standard"
	ModeStrict   LockMode = "strict"
)

type LockKind string

const (
	KindParent LockKind = "parent"
	KindChild  LockKind = "child"
)

type ExecutionKind string

const (
	ExecutionSync  ExecutionKind = "sync"
	ExecutionAsync ExecutionKind = "async"
	ExecutionBoth  ExecutionKind = "both"
)

type BackendFailurePolicy string

const (
	BackendFailClosed   BackendFailurePolicy = "fail_closed"
	BackendBestEffortOpen BackendFailurePolicy = "best_effort_open"
)

type RetryPolicy struct {
	MaxRetries int
}

type KeyBuilder interface {
	RequiredFields() []string
	Build(input map[string]string) (string, error)
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
	KeyBuilder           KeyBuilder
	Tags                 map[string]string
}
```

```go
type OwnershipMeta struct {
	ServiceName   string
	InstanceID    string
	HandlerName   string
	OwnerID       string
	RequestID     string
	MessageID     string
	Attempt       int
	ConsumerGroup string
}

type RuntimeOverrides struct {
	WaitTimeout *time.Duration
	MaxRetries  *int
}

type SyncLockRequest struct {
	DefinitionID string
	KeyInput     map[string]string
	Ownership    OwnershipMeta
	Overrides    *RuntimeOverrides
}

type PresenceCheckRequest struct {
	DefinitionID string
	KeyInput     map[string]string
	Ownership    OwnershipMeta
}

type LeaseContext struct {
	DefinitionID  string
	ResourceKey   string
	Ownership     OwnershipMeta
	LeaseTTL      time.Duration
	LeaseDeadline time.Time
}

type PresenceState int

const (
	PresenceHeld PresenceState = iota
	PresenceNotHeld
	PresenceUnknown
)

type PresenceStatus struct {
	State         PresenceState
	Mode          LockMode
	OwnerID       string
	LeaseDeadline time.Time
}
```

- [ ] **Step 4: Re-run tests**

Run: `go test ./lockkit/definitions -run KeyBuilder -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lockkit/definitions/types.go lockkit/definitions/key_builder.go lockkit/definitions/ownership.go lockkit/definitions/key_builder_test.go
git commit -m "feat: add lock definitions and key builders"
```

## Task 3: Add Typed Errors And Observability Contracts

**Files:**
- Create: `lockkit/errors/errors.go`
- Create: `lockkit/observe/contracts.go`
- Test: `lockkit/errors/errors_test.go`

- [ ] **Step 1: Write the failing error-behavior tests**

```go
func TestErrReentrantAcquireMatchesWithErrorsIs(t *testing.T) {
	err := fmt.Errorf("runtime rejected acquire: %w", lockerrors.ErrReentrantAcquire)
	if !errors.Is(err, lockerrors.ErrReentrantAcquire) {
		t.Fatal("expected ErrReentrantAcquire to match with errors.Is")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./lockkit/errors -v`
Expected: FAIL with undefined package symbols

- [ ] **Step 3: Implement typed errors and no-op observability contracts**

```go
var (
	ErrLockBusy           = errors.New("lock busy")
	ErrLockAcquireTimeout = errors.New("lock acquire timeout")
	ErrLeaseLost          = errors.New("lease lost")
	ErrRegistryViolation  = errors.New("registry violation")
	ErrPolicyViolation    = errors.New("policy violation")
	ErrReentrantAcquire   = errors.New("reentrant acquire")
)
```

```go
type Recorder interface {
	RecordAcquire(ctx context.Context, definitionID string, wait time.Duration, success bool)
	RecordContention(ctx context.Context, definitionID string)
	RecordTimeout(ctx context.Context, definitionID string)
	RecordActiveLocks(ctx context.Context, definitionID string, count int)
	RecordRelease(ctx context.Context, definitionID string, held time.Duration)
	RecordPresenceCheck(ctx context.Context, definitionID string, duration time.Duration)
}

func NewNoopRecorder() Recorder { ... }
```

- [ ] **Step 4: Run package tests**

Run: `go test ./lockkit/errors ./lockkit/observe -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lockkit/errors/errors.go lockkit/errors/errors_test.go lockkit/observe/contracts.go
git commit -m "feat: add typed errors and observability contracts"
```

## Task 4: Implement Registry Registration And Validation

**Files:**
- Create: `lockkit/registry/registry.go`
- Create: `lockkit/registry/validation.go`
- Test: `lockkit/registry/registry_test.go`

- [ ] **Step 1: Write the failing registry tests**

```go
func TestRegistryRejectsDuplicateDefinitionIDs(t *testing.T) {
	reg := registry.New()

	def := definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}"),
	}

	if err := reg.Register(def); err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	if err := reg.Register(def); err == nil {
		t.Fatal("expected duplicate ID rejection")
	}
}

func TestRegistryValidateRejectsStrictWithoutFencing(t *testing.T) {
	reg := registry.New()
	err := reg.Register(definitions.LockDefinition{
		ID:               "PaymentLock",
		Kind:             definitions.KindParent,
		Resource:         "payment",
		Mode:             definitions.ModeStrict,
		ExecutionKind:    definitions.ExecutionSync,
		KeyBuilder:       definitions.MustTemplateKeyBuilder("payment:{payment_id}"),
		FencingRequired:  false,
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected strict validation failure")
	}
}

func TestRegistryValidateRejectsDefinitionWithoutKeyBuilder(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "BrokenLock",
		Kind:          definitions.KindParent,
		Resource:      "broken",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected missing key builder validation failure")
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `go test ./lockkit/registry -v`
Expected: FAIL with undefined registry package or methods

- [ ] **Step 3: Implement registry storage and validation**

```go
type Reader interface {
	MustGet(id string) definitions.LockDefinition
}

type Registry struct {
	definitions map[string]definitions.LockDefinition
}

func (r *Registry) Register(def definitions.LockDefinition) error { ... }
func (r *Registry) MustGet(id string) definitions.LockDefinition { ... }
func (r *Registry) Validate() error { ... }
```

- [ ] **Step 4: Run tests**

Run: `go test ./lockkit/registry -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lockkit/registry/registry.go lockkit/registry/validation.go lockkit/registry/registry_test.go
git commit -m "feat: add lock registry and validation"
```

## Task 5: Define Driver Contract And Build In-Memory Driver

**Files:**
- Create: `lockkit/drivers/contracts.go`
- Create: `lockkit/testkit/memory_driver.go`
- Create: `lockkit/testkit/assertions.go`
- Test: `lockkit/testkit/memory_driver_test.go`

- [ ] **Step 1: Write the failing in-memory driver tests**

```go
func TestMemoryDriverAcquireAndRelease(t *testing.T) {
	driver := testkit.NewMemoryDriver()

	lease, err := driver.Acquire(context.Background(), drivers.AcquireRequest{
		DefinitionID: "OrderLock",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "svc-a:instance-1",
		LeaseTTL:     30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	if len(lease.ResourceKeys) != 1 || lease.ResourceKeys[0] != "order:123" {
		t.Fatalf("unexpected lease keys: %#v", lease.ResourceKeys)
	}

	if err := driver.Release(context.Background(), lease); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `go test ./lockkit/testkit -v`
Expected: FAIL with undefined driver contracts

- [ ] **Step 3: Implement driver interface and memory backend**

```go
type Driver interface {
	Acquire(ctx context.Context, req AcquireRequest) (LeaseRecord, error)
	Renew(ctx context.Context, lease LeaseRecord) (LeaseRecord, error)
	Release(ctx context.Context, lease LeaseRecord) error
	CheckPresence(ctx context.Context, req PresenceRequest) (PresenceRecord, error)
	Ping(ctx context.Context) error
}
```

Driver contract note:
- single-resource execution still uses `ResourceKeys` with exactly one entry
- composite execution is deferred to Phase 2, but the driver shape stays future-compatible

```go
type MemoryDriver struct {
	mu     sync.Mutex
	leases map[string]drivers.LeaseRecord
}
```

- [ ] **Step 4: Re-run driver tests**

Run: `go test ./lockkit/testkit -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lockkit/drivers/contracts.go lockkit/testkit/memory_driver.go lockkit/testkit/assertions.go lockkit/testkit/memory_driver_test.go
git commit -m "feat: add driver contract and in-memory backend"
```

## Task 6: Implement Standard Exclusive Execution

**Files:**
- Create: `lockkit/runtime/manager.go`
- Create: `lockkit/runtime/exclusive.go`
- Test: `lockkit/runtime/exclusive_test.go`

- [ ] **Step 1: Write failing exclusive-execution tests**

```go
func TestExecuteExclusiveRunsCallbackWhenLockAcquired(t *testing.T) {
	reg := registry.New()
	err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}"),
	})
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := runtime.NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	called := false
	err = mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			ServiceName: "svc",
			InstanceID:  "one",
			HandlerName: "UpdateOrder",
			OwnerID:     "svc:one",
		},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		called = true
		if lease.ResourceKey != "order:123" {
			t.Fatalf("unexpected resource key: %q", lease.ResourceKey)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("ExecuteExclusive returned error: %v", err)
	}
	if !called {
		t.Fatal("expected callback to run")
	}
}

func TestExecuteExclusiveRejectsReentrantAcquire(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}"),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := runtime.NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	req := definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			ServiceName: "svc",
			InstanceID:  "one",
			HandlerName: "UpdateOrder",
			OwnerID:     "svc:one",
		},
	}

	err = mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		return mgr.ExecuteExclusive(ctx, req, func(ctx context.Context, nested definitions.LeaseContext) error {
			return nil
		})
	})

	if !errors.Is(err, lockerrors.ErrReentrantAcquire) {
		t.Fatalf("expected reentrant acquire error, got %v", err)
	}
}

func TestExecuteExclusiveHonorsContextDeadlineBeforeWaitTimeout(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		WaitTimeout:   5 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}"),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	driver := testkit.NewMemoryDriver()
	heldLease, err := driver.Acquire(context.Background(), drivers.AcquireRequest{
		DefinitionID: "OrderLock",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "svc:other",
		LeaseTTL:     30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}
	defer driver.Release(context.Background(), heldLease)

	mgr, err := runtime.NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = mgr.ExecuteExclusive(ctx, definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected timeout or context cancellation")
	}
	if time.Since(start) >= time.Second {
		t.Fatal("expected context deadline to beat wait timeout")
	}
}
```

- [ ] **Step 2: Run the runtime tests**

Run: `go test ./lockkit/runtime -run ExecuteExclusive -v`
Expected: FAIL with undefined manager symbols

- [ ] **Step 3: Implement manager and exclusive flow**

```go
type Manager struct {
	registry registry.Reader
	driver   drivers.Driver
	recorder observe.Recorder
	active   sync.Map
}

func NewManager(reg registry.Reader, driver drivers.Driver, recorder observe.Recorder) (*Manager, error) { ... }

func (m *Manager) ExecuteExclusive(
	ctx context.Context,
	req definitions.SyncLockRequest,
	fn func(context.Context, definitions.LeaseContext) error,
) error { ... }
```

Implementation notes:
- `NewManager` should validate the registry before returning
- reject reentrant acquire before hitting the driver
- honor context deadline before acquire
- effective acquire timeout is `min(ctx deadline, WaitTimeout)`
- release in `defer`
- populate `LeaseContext.ResourceKey` for single-lock execution

- [ ] **Step 4: Re-run runtime tests**

Run: `go test ./lockkit/runtime -run ExecuteExclusive -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lockkit/runtime/manager.go lockkit/runtime/exclusive.go lockkit/runtime/exclusive_test.go
git commit -m "feat: add standard exclusive runtime"
```

## Task 7: Harden Validation And Runtime Construction

**Files:**
- Modify: `lockkit/registry/registry_test.go`
- Modify: `lockkit/runtime/exclusive_test.go`

- [ ] **Step 1: Write failing tests for remaining Phase 1 validation coverage**

```go
func TestRegistryValidateRejectsInvalidFailOpenStrictDefinition(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:                   "PaymentLock",
		Kind:                 definitions.KindParent,
		Resource:             "payment",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionSync,
		BackendFailurePolicy: definitions.BackendBestEffortOpen,
		FencingRequired:      true,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("payment:{payment_id}"),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	if err := reg.Validate(); err == nil {
		t.Fatal("expected strict fail-open validation failure")
	}
}

func TestNewManagerRejectsInvalidRegistry(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "BrokenLock",
		Kind:          definitions.KindParent,
		Resource:      "broken",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err := runtime.NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err == nil {
		t.Fatal("expected invalid registry rejection")
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `go test ./lockkit/registry ./lockkit/runtime -run 'Validate|NewManager' -v`
Expected: FAIL with missing validation or constructor behavior

- [ ] **Step 3: Implement the minimal validation and constructor checks**

Implementation notes:
- reject strict definitions that opt into fail-open behavior
- make `NewManager` fail when the registry cannot validate
- keep composite-specific validation out of Phase 1

- [ ] **Step 4: Re-run the focused tests**

Run: `go test ./lockkit/registry ./lockkit/runtime -run 'Validate|NewManager' -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add lockkit/registry/registry_test.go lockkit/runtime/exclusive_test.go
git commit -m "test: harden phase 1 validation and manager construction"
```

## Task 8: Implement Presence Checks And Shutdown Semantics

**Files:**
- Create: `lockkit/runtime/presence.go`
- Create: `lockkit/runtime/metrics.go`
- Test: `lockkit/runtime/presence_test.go`
- Test: `lockkit/runtime/shutdown_test.go`

- [ ] **Step 1: Write failing presence and shutdown tests**

```go
func TestCheckPresenceReturnsPresenceHeld(t *testing.T) {
	driver := testkit.NewMemoryDriver()
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}"),
		CheckOnlyAllowed: true,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := runtime.NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	_, err = driver.Acquire(context.Background(), drivers.AcquireRequest{
		DefinitionID: "OrderLock",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "svc:one",
		LeaseTTL:     30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	status, err := mgr.CheckPresence(context.Background(), definitions.PresenceCheckRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}
	if status.State != definitions.PresenceHeld {
		t.Fatalf("expected PresenceHeld, got %v", status.State)
	}
}

func TestCheckPresenceRejectsDefinitionWithoutCheckOnlyAllowed(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:               "OrderLock",
		Kind:             definitions.KindParent,
		Resource:         "order",
		Mode:             definitions.ModeStandard,
		ExecutionKind:    definitions.ExecutionSync,
		KeyBuilder:       definitions.MustTemplateKeyBuilder("order:{order_id}"),
		CheckOnlyAllowed: false,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := runtime.NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	_, err = mgr.CheckPresence(context.Background(), definitions.PresenceCheckRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	})
	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation for check-only disabled, got %v", err)
	}
}

func TestShutdownStopsNewAcquisitions(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}"),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := runtime.NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	err = mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		return nil
	})
	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation after shutdown, got %v", err)
	}
}
```

- [ ] **Step 2: Run the tests**

Run: `go test ./lockkit/runtime -run 'Presence|Shutdown' -v`
Expected: FAIL with undefined methods

- [ ] **Step 3: Implement presence checks and lifecycle shutdown**

```go
func (m *Manager) CheckPresence(
	ctx context.Context,
	req definitions.PresenceCheckRequest,
) (definitions.PresenceStatus, error) { ... }

func (m *Manager) Shutdown(ctx context.Context) error { ... }
```

Implementation notes:
- return `PresenceUnknown` when driver health is unavailable
- record presence-check metrics
- make `Shutdown` idempotent
- reject new acquisitions after shutdown starts with `ErrPolicyViolation`

- [ ] **Step 4: Run full test suite**

Run: `go test ./... -cover`
Expected: PASS and non-zero coverage for `definitions`, `registry`, `testkit`, and `runtime`

- [ ] **Step 5: Commit**

```bash
git add lockkit/runtime/presence.go lockkit/runtime/metrics.go lockkit/runtime/presence_test.go lockkit/runtime/shutdown_test.go
git commit -m "feat: add presence checks and lifecycle shutdown"
```

## Task 9: Final Documentation Pass

**Files:**
- Modify: `README.md`
- Modify: `docs/superpowers/specs/2026-03-26-lock-management-platform-design.md`

- [ ] **Step 1: Write the failing documentation check**

Run: `rg -n "Phase 1|ExecuteExclusive|CheckPresence" README.md docs/superpowers/specs/2026-03-26-lock-management-platform-design.md`
Expected: missing or incomplete references to implemented Phase 1 API surface

- [ ] **Step 2: Update docs to match the actual Phase 1 deliverable**

```md
## Phase 1 status

- standard-mode exclusive execution
- presence checks
- in-memory driver
- central registry and validation
```

- [ ] **Step 3: Verify docs mention the shipped API**

Run: `rg -n "ExecuteExclusive|CheckPresence|Shutdown" README.md docs/superpowers/specs/2026-03-26-lock-management-platform-design.md`
Expected: matches in both files

- [ ] **Step 4: Run the full verification one last time**

Run: `go test ./... -cover`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add README.md docs/superpowers/specs/2026-03-26-lock-management-platform-design.md
git commit -m "docs: document phase 1 lock platform foundation"
```

## Execution Notes

- Keep tasks in order. Later tasks assume earlier packages and contracts already exist.
- Do not start strict mode, worker flows, or repository guards in this plan.
- Prefer the standard library test runner over adding external test dependencies unless a concrete gap appears.
- If the module path must change from `lockman`, update `go.mod` first and keep imports consistent before continuing.

## Follow-On Plans

After this plan is complete and passing:

1. Phase 2 plan: workers, idempotency contracts, first production driver, child/composite hardening
2. Phase 3a plan: strict mode and fencing
3. Phase 3b plan: guarded write contracts and repository helpers
4. Phase 3c plan: tracing, audit hooks, introspection
