# Performance Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate unnecessary CPU and allocation overhead on the lockman SDK hot path across all three layers: SDK core, Redis driver, and concurrency primitives.

**Architecture:** Changes are internal-only — the public API (`client.Run`, `client.Claim`, `Binding`) stays frozen. Work is delivered in three independent layers, each with its own commit(s). Each layer's existing tests must pass before moving to the next.

**Tech Stack:** Go 1.22+, `sync/atomic`, `strings.Builder`, `testing.B` benchmarks

**Spec:** `docs/superpowers/specs/2026-04-01-performance-design.md`

---

## File Map

### Layer 1: SDK Core

| Action | File                                   | Responsibility                                        |
| ------ | -------------------------------------- | ----------------------------------------------------- |
| Modify | `lockkit/registry/registry.go`         | Add `Get` and `GetComposite` non-cloning methods      |
| Modify | `lockkit/runtime/manager.go`           | Add `activeByDef` counters, cached lineage/defs maps  |
| Modify | `lockkit/runtime/exclusive.go`         | Use `Get`, atomic counters, cached lineage check      |
| Modify | `lockkit/workers/manager.go`           | Use `Get`, cached lineage/defs maps                   |
| Modify | `lockkit/workers/execute.go`           | Use cached lineage check, remove `definitionsByID()`  |
| Modify | `lockkit/workers/execute_composite.go` | Use `GetComposite` instead of `MustGetComposite`      |
| Modify | `lockkit/definitions/key_builder.go`   | Pre-compute placeholders, fast single-field path      |
| Create | `benchmarks/benchmark_layer1_test.go`  | New benchmarks for activeCount, plan build, key build |

### Layer 2: Redis Driver

| Action | File                              | Responsibility                                 |
| ------ | --------------------------------- | ---------------------------------------------- |
| Modify | `backend/redis/driver.go`         | Cached definition ID encoding, `+` concat keys |
| Modify | `backend/redis/scripts.go`        | Move PEXPIRE in lineageRenewScript             |
| Create | `backend/redis/benchmark_test.go` | Key-building benchmark                         |

### Layer 3: Concurrency Primitives

| Action | File                          | Responsibility                   |
| ------ | ----------------------------- | -------------------------------- |
| Modify | `lockkit/runtime/manager.go`  | Atomic in-flight counter         |
| Modify | `lockkit/workers/manager.go`  | Atomic in-flight counter         |
| Modify | `lockkit/workers/shutdown.go` | Adapt shutdown to atomic pattern |
| Modify | `idempotency/memory_store.go` | Sharded mutex                    |

---

## Task 1: Add `Get` and `GetComposite` to Registry

**Files:**

- Modify: `lockkit/registry/registry.go:163-183`
- Modify: `lockkit/registry/registry.go:12-15` (Reader interface)

- [ ] **Step 1: Write failing tests for `Get` and `GetComposite`**

Add to `lockkit/registry/registry_test.go`:

```go
func TestGetReturnsDefinitionWithoutClone(t *testing.T) {
	reg := registry.New()
	tags := map[string]string{"env": "prod"}
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		Tags:          tags,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := reg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	def, ok := reg.Get("OrderLock")
	if !ok {
		t.Fatal("expected Get to return true")
	}
	if def.ID != "OrderLock" {
		t.Fatalf("expected ID OrderLock, got %s", def.ID)
	}

	_, ok = reg.Get("nonexistent")
	if ok {
		t.Fatal("expected Get to return false for unknown ID")
	}
}

func TestGetCompositeReturnsDefinitionWithoutClone(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "alpha",
		Kind:          definitions.KindParent,
		Resource:      "alpha",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("alpha:{id}", []string{"id"}),
	}); err != nil {
		t.Fatalf("register member: %v", err)
	}
	if err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:      "transfer",
		Members: []string{"alpha"},
	}); err != nil {
		t.Fatalf("register composite: %v", err)
	}

	def, ok := reg.GetComposite("transfer")
	if !ok {
		t.Fatal("expected GetComposite to return true")
	}
	if def.ID != "transfer" {
		t.Fatalf("expected ID transfer, got %s", def.ID)
	}

	_, ok = reg.GetComposite("nonexistent")
	if ok {
		t.Fatal("expected GetComposite to return false for unknown ID")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd lockkit/registry && go test -run 'TestGet(Returns|Composite)' -v`
Expected: compilation error — `Get` and `GetComposite` methods don't exist.

- [ ] **Step 3: Implement `Get` and `GetComposite`**

In `lockkit/registry/registry.go`, add the `Get` and `GetComposite` methods to the `Reader` interface and implement them:

Update the `Reader` interface:

```go
type Reader interface {
	MustGet(id string) definitions.LockDefinition
	MustGetComposite(id string) definitions.CompositeDefinition
	Get(id string) (definitions.LockDefinition, bool)
	GetComposite(id string) (definitions.CompositeDefinition, bool)
	Definitions() []definitions.LockDefinition
}
```

Add the implementations (no cloning — definitions are immutable after Validate):

```go
// Get returns the stored definition without cloning. Safe after Validate().
func (r *Registry) Get(id string) (definitions.LockDefinition, bool) {
	r.mu.RLock()
	def, exists := r.definitions[id]
	r.mu.RUnlock()
	return def, exists
}

// GetComposite returns the stored composite definition without cloning. Safe after Validate().
func (r *Registry) GetComposite(id string) (definitions.CompositeDefinition, bool) {
	r.mu.RLock()
	def, exists := r.composites[id]
	r.mu.RUnlock()
	return def, exists
}
```

Also update the `aliasRegistry` test double in `lockkit/runtime/presence_test.go` to satisfy the updated `Reader` interface:

```go
func (a aliasRegistry) Get(id string) (definitions.LockDefinition, bool) {
	def, ok := a.defs[id]
	return def, ok
}

func (a aliasRegistry) GetComposite(id string) (definitions.CompositeDefinition, bool) {
	return definitions.CompositeDefinition{}, false
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd lockkit/registry && go test -v`
Expected: all tests pass including the two new ones.

Run: `cd lockkit && go build ./...`
Expected: no compilation errors (interface satisfaction check).

- [ ] **Step 5: Commit**

```bash
git add lockkit/registry/registry.go lockkit/registry/registry_test.go lockkit/runtime/presence_test.go
git commit -m "feat(registry): add Get and GetComposite non-cloning read methods"
```

---

## Task 2: Replace `activeCount()` O(N) scan with atomic counters

**Files:**

- Modify: `lockkit/runtime/manager.go:17-27`
- Modify: `lockkit/runtime/exclusive.go:104-105,120-121,207-226`

- [ ] **Step 1: Write the benchmark**

Create `benchmarks/benchmark_layer1_test.go`:

```go
package lockman_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/lockkit/testkit"
)

func BenchmarkActiveCountParallel(b *testing.B) {
	for _, concurrency := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("goroutines=%d", concurrency), func(b *testing.B) {
			uc := benchmarkRunUseCase("bench.active-count")
			reg := lockman.NewRegistry()
			registerBenchmarkRunUseCase(b, reg, uc)

			client, err := lockman.New(
				lockman.WithRegistry(reg),
				lockman.WithIdentity(lockman.Identity{OwnerID: "bench-runner"}),
				lockman.WithBackend(testkit.NewMemoryDriver()),
			)
			if err != nil {
				b.Fatalf("New: %v", err)
			}
			defer client.Shutdown(context.Background())

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					key := fmt.Sprintf("resource-%d", i)
					i++
					req, err := uc.With(key)
					if err != nil {
						b.Fatalf("With: %v", err)
					}
					if err := client.Run(context.Background(), req, func(context.Context, lockman.Lease) error {
						return nil
					}); err != nil {
						b.Fatalf("Run: %v", err)
					}
				}
			})
		})
	}
}
```

- [ ] **Step 2: Run benchmark to establish baseline**

Run: `cd benchmarks && go test -run '^$' -bench 'BenchmarkActiveCountParallel' -benchmem -count=3`
Expected: benchmark runs and produces ns/op, B/op, allocs/op numbers.

- [ ] **Step 3: Replace `activeCount` with atomic counters**

In `lockkit/runtime/manager.go`, add the `activeByDef` field to the Manager struct:

```go
type Manager struct {
	registry      registry.Reader
	driver        backend.Driver
	recorder      observe.Recorder
	active        sync.Map
	activeByDef   sync.Map // definitionID → *atomic.Int64
	shuttingDown  atomic.Bool
	shutdownStart sync.Once
	lifecycleMu   sync.Mutex
	inFlight      int
	inFlightDrain chan struct{}
}
```

Add a helper to get or create the counter:

```go
func (m *Manager) activeCounter(definitionID string) *atomic.Int64 {
	if v, ok := m.activeByDef.Load(definitionID); ok {
		return v.(*atomic.Int64)
	}
	counter := &atomic.Int64{}
	actual, _ := m.activeByDef.LoadOrStore(definitionID, counter)
	return actual.(*atomic.Int64)
}
```

In `lockkit/runtime/exclusive.go`, replace the `activeCount` and `recordActiveLocks` functions:

```go
func (m *Manager) recordActiveLocks(ctx context.Context, definitionID string) {
	count := int(m.activeCounter(definitionID).Load())
	m.recorder.RecordActiveLocks(ctx, definitionID, count)
}
```

Delete the old `activeCount` method (lines 212-226).

In `ExecuteExclusive`, increment after acquire and decrement in deferred cleanup. Replace line 120-121:

```go
	m.active.Store(key, guardEntry{state: guardHeld})
	m.activeCounter(def.ID).Add(1)
	m.recordActiveLocks(ctx, def.ID)
```

In the deferred cleanup (around line 104-105), add the decrement:

```go
	if guardInstalled {
		if v, ok := m.active.Load(key); ok {
			if entry, entryOk := v.(guardEntry); entryOk && entry.state == guardHeld {
				m.activeCounter(key.definitionID).Add(-1)
			}
		}
		m.active.Delete(key)
		m.recordActiveLocks(ctx, def.ID)
	}
```

Apply the same pattern to `ExecuteCompositeExclusive` in `lockkit/runtime/composite.go`:

In the composite acquire loop (around line 144):

```go
	m.active.Store(guardKeys[i], guardEntry{state: guardHeld})
	m.activeCounter(member.Definition.ID).Add(1)
	m.recordActiveLocks(ctx, member.Definition.ID)
```

In the composite deferred cleanup (around line 99-104):

```go
	if !guardInstalled {
		return
	}
	for _, key := range guardKeys {
		if v, ok := m.active.Load(key); ok {
			if entry, entryOk := v.(guardEntry); entryOk && entry.state == guardHeld {
				m.activeCounter(key.definitionID).Add(-1)
			}
		}
		m.active.Delete(key)
		m.recordActiveLocks(ctx, key.definitionID)
	}
```

- [ ] **Step 4: Run all runtime tests**

Run: `cd lockkit/runtime && go test -v -count=1`
Expected: all existing tests pass.

- [ ] **Step 5: Re-run benchmark to verify improvement**

Run: `cd benchmarks && go test -run '^$' -bench 'BenchmarkActiveCountParallel' -benchmem -count=3`
Expected: lower ns/op, especially at higher goroutine counts.

- [ ] **Step 6: Commit**

```bash
git add lockkit/runtime/manager.go lockkit/runtime/exclusive.go lockkit/runtime/composite.go benchmarks/benchmark_layer1_test.go
git commit -m "perf(runtime): replace activeCount O(N) scan with atomic counters"
```

---

## Task 3: Cache lineage check and definitions-by-ID map

**Files:**

- Modify: `lockkit/runtime/manager.go:17-27,30-63`
- Modify: `lockkit/runtime/exclusive.go:238-257,347-369`
- Modify: `lockkit/workers/manager.go:26-42,45-85`
- Modify: `lockkit/workers/execute.go:354-373,464-486`

- [ ] **Step 1: Add cached fields to runtime Manager**

In `lockkit/runtime/manager.go`, add cached fields to the Manager struct:

```go
type Manager struct {
	registry      registry.Reader
	driver        backend.Driver
	recorder      observe.Recorder
	active        sync.Map
	activeByDef   sync.Map
	lineageDefs   map[string]bool
	cachedDefsByID map[string]definitions.LockDefinition
	shuttingDown  atomic.Bool
	shutdownStart sync.Once
	lifecycleMu   sync.Mutex
	inFlight      int
	inFlightDrain chan struct{}
}
```

In `NewManager`, after validation succeeds, build the cached structures:

```go
	defs := reg.Definitions()
	defsByID := make(map[string]definitions.LockDefinition, len(defs))
	for _, def := range defs {
		defsByID[def.ID] = def
	}
	childrenByParent := make(map[string][]string, len(defs))
	for _, def := range defs {
		if def.ParentRef == "" {
			continue
		}
		childrenByParent[def.ParentRef] = append(childrenByParent[def.ParentRef], def.ID)
	}
	lineageDefs := make(map[string]bool, len(defs))
	for _, def := range defs {
		lineageDefs[def.ID] = def.ParentRef != "" || len(childrenByParent[def.ID]) > 0
	}

	return &Manager{
		registry:       reg,
		driver:         driver,
		recorder:       recorder,
		lineageDefs:    lineageDefs,
		cachedDefsByID: defsByID,
		inFlightDrain: func() chan struct{} {
			ch := make(chan struct{})
			close(ch)
			return ch
		}(),
	}, nil
```

- [ ] **Step 2: Replace `buildAcquirePlan` to use cached data**

In `lockkit/runtime/exclusive.go`, replace `buildAcquirePlan`:

```go
func (m *Manager) buildAcquirePlan(def definitions.LockDefinition, input map[string]string) (runtimeAcquirePlan, error) {
	if !m.lineageDefs[def.ID] {
		resourceKey, err := def.KeyBuilder.Build(input)
		if err != nil {
			return runtimeAcquirePlan{}, err
		}
		return runtimeAcquirePlan{resourceKey: resourceKey}, nil
	}

	plan, err := lineage.ResolveAcquirePlan(def, m.cachedDefsByID, input)
	if err != nil {
		return runtimeAcquirePlan{}, err
	}
	meta := plan.LeaseMeta()
	return runtimeAcquirePlan{
		resourceKey: plan.ResourceKey,
		lineage:     &meta,
	}, nil
}
```

Delete the now-unused `definitionsByID()`, `childrenByParent()`, and `runtimeDefinitionUsesLineage()` functions from `exclusive.go`.

- [ ] **Step 3: Apply the same pattern to workers Manager**

In `lockkit/workers/manager.go`, add cached fields:

```go
type Manager struct {
	registry       registry.Reader
	driver         backend.Driver
	idempotency    idempotency.Store
	active         sync.Map
	lineageDefs    map[string]bool
	cachedDefsByID map[string]definitions.LockDefinition
	shuttingDown   atomic.Bool
	shutdownStart  sync.Once
	lifecycleMu    sync.Mutex
	inFlight       int
	inFlightDrain  chan struct{}
	renewalsMu     sync.Mutex
	renewals       map[uint64]context.CancelFunc
	nextRenewal    uint64
}
```

In `workers.NewManager`, after validation succeeds, build the same cached structures (before the return):

```go
	defs := reg.Definitions()
	defsByID := make(map[string]definitions.LockDefinition, len(defs))
	for _, def := range defs {
		defsByID[def.ID] = def
	}
	childrenByParent := make(map[string][]string, len(defs))
	for _, def := range defs {
		if def.ParentRef == "" {
			continue
		}
		childrenByParent[def.ParentRef] = append(childrenByParent[def.ParentRef], def.ID)
	}
	lineageDefs := make(map[string]bool, len(defs))
	for _, def := range defs {
		lineageDefs[def.ID] = def.ParentRef != "" || len(childrenByParent[def.ID]) > 0
	}
```

Update the returned `&Manager{...}` to include the new fields (while keeping existing fields like `inFlightDrain`):

```go
	drain := make(chan struct{})
	close(drain)
	return &Manager{
		registry:       reg,
		driver:         driver,
		idempotency:    store,
		lineageDefs:    lineageDefs,
		cachedDefsByID: defsByID,
		inFlightDrain:  drain,
		renewals:       make(map[uint64]context.CancelFunc),
	}, nil
```

In `lockkit/workers/execute.go`, replace `buildClaimAcquirePlan`:

```go
func (m *Manager) buildClaimAcquirePlan(def definitions.LockDefinition, input map[string]string) (claimAcquirePlan, error) {
	if !m.lineageDefs[def.ID] {
		resourceKey, err := def.KeyBuilder.Build(input)
		if err != nil {
			return claimAcquirePlan{}, err
		}
		return claimAcquirePlan{resourceKey: resourceKey}, nil
	}

	plan, err := lineage.ResolveAcquirePlan(def, m.cachedDefsByID, input)
	if err != nil {
		return claimAcquirePlan{}, err
	}
	meta := plan.LeaseMeta()
	return claimAcquirePlan{
		resourceKey: plan.ResourceKey,
		lineage:     &meta,
	}, nil
}
```

Delete the now-unused `definitionsByID()`, `workerChildrenByParent()`, and `workerDefinitionUsesLineage()` functions from `execute.go`.

- [ ] **Step 4: Run all tests**

Run: `cd lockkit && go test ./... -count=1`
Expected: all tests pass in both `runtime` and `workers` packages.

- [ ] **Step 5: Commit**

```bash
git add lockkit/runtime/manager.go lockkit/runtime/exclusive.go lockkit/workers/manager.go lockkit/workers/execute.go
git commit -m "perf(sdk): cache lineage check and definitions-by-ID at init time"
```

---

## Task 4: Replace `defer/recover` with `Get` in both managers

**Files:**

- Modify: `lockkit/runtime/exclusive.go:228-236`
- Modify: `lockkit/runtime/manager.go:123-131`
- Modify: `lockkit/workers/manager.go:175-183`
- Modify: `lockkit/workers/execute_composite.go:291-299`

- [ ] **Step 1: Replace `getDefinition` in runtime Manager**

In `lockkit/runtime/exclusive.go`, replace:

```go
func (m *Manager) getDefinition(id string) (definitions.LockDefinition, bool) {
	return m.registry.Get(id)
}
```

Update all callers of `getDefinition` in `exclusive.go` to handle the `bool` return. In `ExecuteExclusive` (around line 52):

```go
	def, ok := m.getDefinition(req.DefinitionID)
	if !ok {
		return lockerrors.ErrPolicyViolation
	}
```

- [ ] **Step 2: Replace `getCompositeDefinition` in runtime Manager**

In `lockkit/runtime/manager.go`, replace:

```go
func (m *Manager) getCompositeDefinition(id string) (definitions.CompositeDefinition, bool) {
	return m.registry.GetComposite(id)
}
```

Update the caller in `composite.go` `ExecuteCompositeExclusive` (around line 28):

```go
	compositeDef, ok := m.getCompositeDefinition(req.DefinitionID)
	if !ok {
		return lockerrors.ErrPolicyViolation
	}
```

- [ ] **Step 3: Replace `getDefinition` in workers Manager**

In `lockkit/workers/manager.go`, replace:

```go
func (m *Manager) getDefinition(id string) (definitions.LockDefinition, bool) {
	return m.registry.Get(id)
}
```

Update the caller in `execute.go` `ExecuteClaimed` (around line 47):

```go
	def, ok := m.getDefinition(req.DefinitionID)
	if !ok {
		return lockerrors.ErrPolicyViolation
	}
```

- [ ] **Step 4: Replace `getCompositeDefinition` in workers**

In `lockkit/workers/execute_composite.go`, replace:

```go
func (m *Manager) getCompositeDefinition(id string) (definitions.CompositeDefinition, bool) {
	return m.registry.GetComposite(id)
}
```

Update its caller in the same file to handle the `bool` return.

- [ ] **Step 5: Run all tests**

Run: `cd lockkit && go test ./... -count=1`
Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add lockkit/runtime/exclusive.go lockkit/runtime/manager.go lockkit/runtime/composite.go lockkit/workers/manager.go lockkit/workers/execute.go lockkit/workers/execute_composite.go
git commit -m "perf(sdk): replace defer/recover with Get in definition lookups"
```

---

## Task 5: Optimize `TemplateKeyBuilder.Build`

**Files:**

- Modify: `lockkit/definitions/key_builder.go:31-34,87-102`

- [ ] **Step 1: Write benchmarks**

Add to `benchmarks/benchmark_layer1_test.go`:

```go
func BenchmarkBuildAcquirePlan(b *testing.B) {
	uc := benchmarkRunUseCase("bench.acquire-plan")
	reg := lockman.NewRegistry()
	registerBenchmarkRunUseCase(b, reg, uc)

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "bench-runner"}),
		lockman.WithBackend(testkit.NewMemoryDriver()),
	)
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	defer client.Shutdown(context.Background())

	req, err := uc.With("order-123")
	if err != nil {
		b.Fatalf("With: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := client.Run(context.Background(), req, func(context.Context, lockman.Lease) error {
			return nil
		}); err != nil {
			b.Fatalf("Run: %v", err)
		}
	}
}

func BenchmarkKeyBuilderBuildSingle(b *testing.B) {
	builder := definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"})
	input := map[string]string{"order_id": "12345"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := builder.Build(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkKeyBuilderBuildMulti(b *testing.B) {
	builder := definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"})
	input := map[string]string{"order_id": "12345", "item_id": "789"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := builder.Build(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}
```

- [ ] **Step 2: Run benchmarks to establish baseline**

Run: `cd benchmarks && go test -run '^$' -bench 'BenchmarkKeyBuilderBuild' -benchmem -count=3`
Expected: baseline numbers recorded.

- [ ] **Step 3: Optimize the builder**

In `lockkit/definitions/key_builder.go`, pre-compute placeholder strings at construction and add a fast path for single-field templates:

```go
type templateKeyBuilder struct {
	template     string
	fields       []string
	placeholders []string // pre-computed: "{field_name}"
}
```

Update `NewTemplateKeyBuilder` to pre-compute placeholders:

```go
	placeholders := make([]string, len(ordered))
	for i, field := range ordered {
		placeholders[i] = "{" + field + "}"
	}

	fieldsCopy := make([]string, len(ordered))
	copy(fieldsCopy, ordered)
	return &templateKeyBuilder{
		template:     template,
		fields:       fieldsCopy,
		placeholders: placeholders,
	}, nil
```

Replace the `Build` method:

```go
func (t *templateKeyBuilder) Build(input map[string]string) (string, error) {
	if input == nil {
		return "", fmt.Errorf("input map must not be nil")
	}

	// Fast path: single field — use strings.Replace directly
	if len(t.fields) == 1 {
		value, ok := input[t.fields[0]]
		if !ok {
			return "", fmt.Errorf("missing required field: %s", t.fields[0])
		}
		return strings.Replace(t.template, t.placeholders[0], value, 1), nil
	}

	// Multi-field: build replacer from pre-computed placeholders
	replacements := make([]string, 0, len(t.fields)*2)
	for i, field := range t.fields {
		value, ok := input[field]
		if !ok {
			return "", fmt.Errorf("missing required field: %s", field)
		}
		replacements = append(replacements, t.placeholders[i], value)
	}
	return strings.NewReplacer(replacements...).Replace(t.template), nil
}
```

- [ ] **Step 4: Run key builder tests**

Run: `cd lockkit/definitions && go test -v -count=1`
Expected: all existing tests pass.

- [ ] **Step 5: Re-run benchmarks**

Run: `cd benchmarks && go test -run '^$' -bench 'BenchmarkKeyBuilderBuild' -benchmem -count=3`
Expected: fewer allocs/op, especially for single-field case.

- [ ] **Step 6: Commit**

```bash
git add lockkit/definitions/key_builder.go benchmarks/benchmark_layer1_test.go
git commit -m "perf(definitions): optimize TemplateKeyBuilder.Build with pre-computed placeholders"
```

---

## Task 6: Run full Layer 1 validation

- [ ] **Step 1: Run all unit tests**

Run: `go test ./... -count=1`
Expected: all tests pass.

- [ ] **Step 2: Run existing adoption benchmarks**

Run: `cd benchmarks && go test -run '^$' -bench '^BenchmarkAdoption' -benchmem -count=3`
Expected: all benchmarks pass, ns/op and allocs/op improved compared to pre-optimization baseline.

- [ ] **Step 3: Commit any missed files**

Run: `git status`
If clean, proceed. If uncommitted changes remain, add and commit them.

---

## Task 7: Cache encoded definition IDs in Redis driver

**Files:**

- Modify: `backend/redis/driver.go:22-39,422-436,516-518`

- [ ] **Step 1: Write benchmark**

Create `backend/redis/benchmark_test.go`:

```go
package redis

import "testing"

func BenchmarkBuildLeaseKey(b *testing.B) {
	drv := &Driver{
		keyPrefix: "lockman:lease",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = drv.buildLeaseKey("order.standard", "order:12345")
	}
}

func BenchmarkBuildLeaseKeyWithCache(b *testing.B) {
	drv := &Driver{
		keyPrefix:  "lockman:lease",
		encodedIDs: map[string]string{},
	}
	drv.cacheDefinitionID("order.standard")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = drv.buildLeaseKey("order.standard", "order:12345")
	}
}
```

Note: this benchmark won't compile yet — step 2 adds the fields.

- [ ] **Step 2: Add `encodedIDs` cache and `cacheDefinitionID` to Driver**

In `backend/redis/driver.go`, add the cache field to the Driver struct:

```go
type Driver struct {
	client     goredis.UniversalClient
	keyPrefix  string
	encodedIDs map[string]string // definitionID → base64-encoded segment
	now        func() time.Time
}
```

Add a method to populate it:

```go
// CacheDefinitionIDs pre-encodes definition IDs for faster key building.
func (d *Driver) CacheDefinitionIDs(ids []string) {
	if d.encodedIDs == nil {
		d.encodedIDs = make(map[string]string, len(ids))
	}
	for _, id := range ids {
		d.encodedIDs[id] = encodeSegment(id)
	}
}

func (d *Driver) cacheDefinitionID(id string) {
	if d.encodedIDs == nil {
		d.encodedIDs = make(map[string]string)
	}
	d.encodedIDs[id] = encodeSegment(id)
}
```

Add a helper that uses the cache:

```go
func (d *Driver) encodeDefinitionID(id string) string {
	if d.encodedIDs != nil {
		if encoded, ok := d.encodedIDs[id]; ok {
			return encoded
		}
	}
	return encodeSegment(id)
}
```

- [ ] **Step 3: Replace `encodeSegment(definitionID)` with `encodeDefinitionID` in all key builders**

```go
func (d *Driver) buildLeaseKey(definitionID, resourceKey string) string {
	return d.keyPrefix + ":" + d.encodeDefinitionID(definitionID) + ":" + encodeSegment(resourceKey)
}

func (d *Driver) buildStrictFenceCounterKey(definitionID, resourceKey string) string {
	return d.keyPrefix + ":fence:" + d.encodeDefinitionID(definitionID) + ":" + encodeSegment(resourceKey)
}

func (d *Driver) buildStrictTokenKey(definitionID, resourceKey string) string {
	return d.keyPrefix + ":strict-token:" + d.encodeDefinitionID(definitionID) + ":" + encodeSegment(resourceKey)
}

func (d *Driver) buildLineageKey(definitionID, resourceKey string) string {
	return d.keyPrefix + ":lineage:" + d.encodeDefinitionID(definitionID) + ":" + encodeSegment(resourceKey)
}
```

- [ ] **Step 4: Run Redis driver tests**

Run: `cd backend/redis && go test -v -count=1`
Expected: all tests pass.

- [ ] **Step 5: Run benchmark**

Run: `cd backend/redis && go test -run '^$' -bench 'BenchmarkBuildLeaseKey' -benchmem -count=3`
Expected: `BenchmarkBuildLeaseKeyWithCache` shows fewer allocs/op than `BenchmarkBuildLeaseKey`.

- [ ] **Step 6: Commit**

```bash
git add backend/redis/driver.go backend/redis/benchmark_test.go
git commit -m "perf(redis): cache encoded definition IDs and use direct concatenation for keys"
```

---

## Task 8: Move PEXPIRE in lineageRenewScript

**Files:**

- Modify: `backend/redis/scripts.go:150-207`

- [ ] **Step 1: Move PEXPIRE after the update loop**

In `backend/redis/scripts.go`, replace the `lineageRenewScript` content. Move the `PEXPIRE` call (currently at line 188) to after the second for-loop:

```go
var lineageRenewScript = goredis.NewScript(`
local ttl = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local member = ARGV[4]
local ancestor_count = tonumber(ARGV[5]) or 0

if not ttl or ttl <= 0 or not now or member == "" then
	return {-3, 0}
end

local current = redis.call("GET", KEYS[1])
if not current then
	return {0, 0}
end
if current ~= ARGV[1] then
	return {-1, 0}
end

local function prune_and_get_remaining(key)
	redis.call("ZREMRANGEBYSCORE", key, "-inf", now)
	local latest = redis.call("ZREVRANGE", key, 0, 0, "WITHSCORES")
	if latest and #latest >= 2 then
		local expiry = tonumber(latest[2])
		if expiry and expiry > now then
			return expiry - now
		end
	end
	return 0
end

local expiry = now + ttl
for i = 1, ancestor_count do
	local lineage_key = KEYS[1 + i]
	if not redis.call("ZSCORE", lineage_key, member) then
		return {-4, 0}
	end
end

for i = 1, ancestor_count do
	local lineage_key = KEYS[1 + i]
	redis.call("ZADD", lineage_key, "XX", expiry, member)
	local remaining = prune_and_get_remaining(lineage_key)
	if remaining <= 0 then
		redis.call("DEL", lineage_key)
	else
		local existing_ttl = redis.call("PTTL", lineage_key)
		if not existing_ttl or existing_ttl < remaining then
			redis.call("PEXPIRE", lineage_key, remaining)
		end
	end
end

if redis.call("PEXPIRE", KEYS[1], ttl) == 0 then
	return {-2, 0}
end

return {1, ttl}
`)
```

- [ ] **Step 2: Run Redis driver tests (including integration)**

Run: `cd backend/redis && go test -v -count=1`
Expected: all tests pass.

- [ ] **Step 3: Commit**

```bash
git add backend/redis/scripts.go
git commit -m "perf(redis): move PEXPIRE after lineage update loop for tighter atomicity"
```

---

## Task 9: Run full Layer 2 validation

- [ ] **Step 1: Run all tests**

Run: `go test ./... -count=1`
Expected: all tests pass.

- [ ] **Step 2: Run adoption benchmarks**

Run: `cd benchmarks && go test -run '^$' -bench '^BenchmarkAdoption' -benchmem -count=3`
Expected: all pass, Redis-backed benchmarks show improvement.

---

## Task 10: Replace `lifecycleMu` with atomics in runtime Manager

**Files:**

- Modify: `lockkit/runtime/manager.go:17-27,58-63,84-121`

- [ ] **Step 1: Replace `inFlight int` with `atomic.Int64` and add `sync.Cond`**

In `lockkit/runtime/manager.go`, update the Manager struct:

```go
type Manager struct {
	registry       registry.Reader
	driver         backend.Driver
	recorder       observe.Recorder
	active         sync.Map
	activeByDef    sync.Map
	lineageDefs    map[string]bool
	cachedDefsByID map[string]definitions.LockDefinition
	shuttingDown   atomic.Bool
	shutdownStart  sync.Once
	inFlight       atomic.Int64
	drainMu        sync.Mutex
	drainCond      *sync.Cond
}
```

Update `NewManager` to initialize the Cond:

```go
	m := &Manager{
		registry:       reg,
		driver:         driver,
		recorder:       recorder,
		lineageDefs:    lineageDefs,
		cachedDefsByID: defsByID,
	}
	m.drainCond = sync.NewCond(&m.drainMu)
	return m, nil
```

Replace the admission functions:

```go
func (m *Manager) tryAdmitInFlightExecution() bool {
	if m.shuttingDown.Load() {
		return false
	}
	m.inFlight.Add(1)
	if m.shuttingDown.Load() {
		m.releaseInFlightExecution()
		return false
	}
	return true
}

func (m *Manager) releaseInFlightExecution() {
	if m.inFlight.Add(-1) == 0 {
		m.drainCond.Broadcast()
	}
}
```

Replace `Shutdown`:

```go
func (m *Manager) Shutdown(ctx context.Context) error {
	m.shutdownStart.Do(func() {
		m.shuttingDown.Store(true)
	})

	done := make(chan struct{})
	go func() {
		m.drainMu.Lock()
		for m.inFlight.Load() > 0 {
			m.drainCond.Wait()
		}
		m.drainMu.Unlock()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		m.drainCond.Broadcast()
		return ctx.Err()
	}
}
```

Delete the `lifecycleMu`, `inFlightDrain`, `inFlightDrainChannel` fields and methods.

- [ ] **Step 2: Run runtime tests**

Run: `cd lockkit/runtime && go test -v -count=1`
Expected: all tests pass.

- [ ] **Step 3: Commit**

```bash
git add lockkit/runtime/manager.go
git commit -m "perf(runtime): replace lifecycleMu with atomic in-flight counter"
```

---

## Task 11: Replace `lifecycleMu` with atomics in workers Manager

**Files:**

- Modify: `lockkit/workers/manager.go:26-42,108-139`
- Modify: `lockkit/workers/shutdown.go`

- [ ] **Step 1: Apply the same atomic pattern**

In `lockkit/workers/manager.go`, update the struct:

```go
type Manager struct {
	registry       registry.Reader
	driver         backend.Driver
	idempotency    idempotency.Store
	active         sync.Map
	lineageDefs    map[string]bool
	cachedDefsByID map[string]definitions.LockDefinition
	shuttingDown   atomic.Bool
	shutdownStart  sync.Once
	inFlight       atomic.Int64
	drainMu        sync.Mutex
	drainCond      *sync.Cond
	renewalsMu     sync.Mutex
	renewals       map[uint64]context.CancelFunc
	nextRenewal    uint64
}
```

Update `NewManager` to initialize:

```go
	m := &Manager{
		registry:       reg,
		driver:         driver,
		idempotency:    store,
		lineageDefs:    lineageDefs,
		cachedDefsByID: defsByID,
		renewals:       make(map[uint64]context.CancelFunc),
	}
	m.drainCond = sync.NewCond(&m.drainMu)
	return m, nil
```

Replace admission functions (same pattern as runtime):

```go
func (m *Manager) tryAdmitInFlightExecution() bool {
	if m.shuttingDown.Load() {
		return false
	}
	m.inFlight.Add(1)
	if m.shuttingDown.Load() {
		m.releaseInFlightExecution()
		return false
	}
	return true
}

func (m *Manager) releaseInFlightExecution() {
	if m.inFlight.Add(-1) == 0 {
		m.drainCond.Broadcast()
	}
}
```

Delete `inFlightDrainChannel` method.

- [ ] **Step 2: Update Shutdown in `lockkit/workers/shutdown.go`**

```go
func (m *Manager) Shutdown(ctx context.Context) error {
	m.shutdownStart.Do(func() {
		m.shuttingDown.Store(true)
	})

	done := make(chan struct{})
	go func() {
		m.drainMu.Lock()
		for m.inFlight.Load() > 0 {
			m.drainCond.Wait()
		}
		m.drainMu.Unlock()
		close(done)
	}()

	select {
	case <-done:
		m.cancelAllRenewals()
		return nil
	case <-ctx.Done():
		m.drainCond.Broadcast()
		return ctx.Err()
	}
}
```

- [ ] **Step 3: Run workers tests**

Run: `cd lockkit/workers && go test -v -count=1`
Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add lockkit/workers/manager.go lockkit/workers/shutdown.go
git commit -m "perf(workers): replace lifecycleMu with atomic in-flight counter"
```

---

## Task 12: Shard `MemoryStore` mutex

**Files:**

- Modify: `idempotency/memory_store.go`

- [ ] **Step 1: Write a concurrency benchmark**

Add to `idempotency/memory_store_test.go`:

```go
func BenchmarkMemoryStoreBeginParallel(b *testing.B) {
	store := NewMemoryStore()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("msg-%d", i)
			i++
			_, _ = store.Begin(context.Background(), key, BeginInput{
				OwnerID:       "worker",
				MessageID:     key,
				ConsumerGroup: "group",
				Attempt:       1,
				TTL:           time.Minute,
			})
		}
	})
}
```

- [ ] **Step 2: Run baseline benchmark**

Run: `cd idempotency && go test -run '^$' -bench 'BenchmarkMemoryStoreBeginParallel' -benchmem -count=3`
Expected: baseline numbers.

- [ ] **Step 3: Implement sharding**

In `idempotency/memory_store.go`, replace the implementation:

```go
import (
	"context"
	"errors"
	"hash/fnv"
	"sync"
	"time"
)

const shardCount = 16

var errInvalidTTL = errors.New("idempotency: invalid ttl")

type memoryShard struct {
	mu      sync.Mutex
	records map[string]Record
}

// MemoryStore is an in-memory idempotency store intended for tests and local runs.
type MemoryStore struct {
	shards [shardCount]memoryShard
	now    func() time.Time
}

func NewMemoryStore() *MemoryStore {
	return NewMemoryStoreWithNow(time.Now)
}

func NewMemoryStoreWithNow(now func() time.Time) *MemoryStore {
	if now == nil {
		now = time.Now
	}
	s := &MemoryStore{now: now}
	for i := range s.shards {
		s.shards[i].records = make(map[string]Record)
	}
	return s
}

func (s *MemoryStore) shard(key string) *memoryShard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return &s.shards[h.Sum32()%shardCount]
}

func (s *MemoryStore) Get(ctx context.Context, key string) (Record, error) {
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}

	sh := s.shard(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	now := s.now()

	record, ok := sh.records[key]
	if !ok || isExpired(record.ExpiresAt, now) {
		if ok {
			delete(sh.records, key)
		}
		return Record{
			Key:    key,
			Status: StatusMissing,
		}, nil
	}

	return record, nil
}

func (s *MemoryStore) Begin(ctx context.Context, key string, input BeginInput) (BeginResult, error) {
	if err := ctx.Err(); err != nil {
		return BeginResult{}, err
	}
	if input.TTL <= 0 {
		return BeginResult{}, errInvalidTTL
	}

	sh := s.shard(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	now := s.now()

	if record, ok := sh.records[key]; ok {
		if !isExpired(record.ExpiresAt, now) {
			return BeginResult{
				Record:    record,
				Acquired:  false,
				Duplicate: true,
			}, nil
		}
		delete(sh.records, key)
	}

	record := Record{
		Key:           key,
		Status:        StatusInProgress,
		OwnerID:       input.OwnerID,
		MessageID:     input.MessageID,
		ConsumerGroup: input.ConsumerGroup,
		Attempt:       input.Attempt,
		UpdatedAt:     now,
		ExpiresAt:     now.Add(input.TTL),
	}
	sh.records[key] = record

	return BeginResult{
		Record:    record,
		Acquired:  true,
		Duplicate: false,
	}, nil
}

func (s *MemoryStore) Complete(ctx context.Context, key string, input CompleteInput) error {
	return s.setTerminalStatus(ctx, key, input.OwnerID, input.MessageID, input.TTL, StatusCompleted)
}

func (s *MemoryStore) Fail(ctx context.Context, key string, input FailInput) error {
	return s.setTerminalStatus(ctx, key, input.OwnerID, input.MessageID, input.TTL, StatusFailed)
}

func (s *MemoryStore) setTerminalStatus(ctx context.Context, key, ownerID, messageID string, ttl time.Duration, status Status) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ttl <= 0 {
		return errInvalidTTL
	}

	sh := s.shard(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	now := s.now()

	record := Record{
		Key:       key,
		Status:    status,
		OwnerID:   ownerID,
		MessageID: messageID,
		UpdatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if existing, ok := sh.records[key]; ok && !isExpired(existing.ExpiresAt, now) {
		record.OwnerID = existing.OwnerID
		record.MessageID = existing.MessageID
		record.ConsumerGroup = existing.ConsumerGroup
		record.Attempt = existing.Attempt
	}

	sh.records[key] = record
	return nil
}

func isExpired(expiresAt, now time.Time) bool {
	return !expiresAt.After(now)
}

var _ Store = (*MemoryStore)(nil)
```

- [ ] **Step 4: Run idempotency tests**

Run: `cd idempotency && go test -v -count=1`
Expected: all existing tests pass.

- [ ] **Step 5: Re-run benchmark**

Run: `cd idempotency && go test -run '^$' -bench 'BenchmarkMemoryStoreBeginParallel' -benchmem -count=3`
Expected: improved ns/op under parallel load.

- [ ] **Step 6: Commit**

```bash
git add idempotency/memory_store.go idempotency/memory_store_test.go
git commit -m "perf(idempotency): shard MemoryStore mutex for concurrent access"
```

---

## Task 13: Final validation

- [ ] **Step 1: Run full test suite**

Run: `go test ./... -count=1`
Expected: all tests pass.

- [ ] **Step 2: Run full benchmark suite**

Run: `cd benchmarks && go test -run '^$' -bench '^BenchmarkAdoption' -benchmem -count=5`
Expected: all benchmarks pass with improved allocations.

- [ ] **Step 3: Run vet and lint**

Run: `go vet ./...`
Expected: no issues.
