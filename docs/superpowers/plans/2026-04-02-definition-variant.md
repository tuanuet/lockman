# Definition + Variant Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow multiple usecases to share a lock key via Definition + Variant API, enabling mutual exclusion across usecases targeting the same resource.

**Architecture:** A generic `Definition[T]` holds a shared `definitionID`. Variants inherit this ID → same Redis lock key. Changes flow through: `useCaseConfig.definitionID` → `normalizeUseCase` → `sdk.NewUseCaseWithID` → `LockDefinition.ID` → backend key construction.

**Tech Stack:** Go 1.22, generics, Redis backend, FNV-64a hashing

---

## Chunk 1: SDK Layer + Config Foundation

### Task 1: Add definitionID to useCaseConfig + newUseCaseCoreWithConfig

**Files:**
- Modify: `binding.go:26-33` (add definitionID field)
- Modify: `registry.go:26-39` (add newUseCaseCoreWithConfig function)

- [ ] **Step 1: Add definitionID to useCaseConfig**

In `binding.go`, add `definitionID string` field to `useCaseConfig`:

```go
type useCaseConfig struct {
	ttl           time.Duration
	wait          time.Duration
	idempotent    bool
	strict        bool
	lineageParent string
	composite     []compositeMemberConfig
	definitionID  string // shared definitionID for variants (empty = derive from name)
}
```

- [ ] **Step 2: Add newUseCaseCoreWithConfig to registry.go**

In `registry.go`, add after `newUseCaseCore`:

```go
func newUseCaseCoreWithConfig(name string, kind useCaseKind, cfg useCaseConfig) *useCaseCore {
	return &useCaseCore{
		name:   strings.TrimSpace(name),
		kind:   kind,
		config: cfg,
	}
}
```

- [ ] **Step 3: Run tests to verify no breakage**

```bash
go test ./... -run '^$'
```

Expected: All packages compile.

- [ ] **Step 4: Commit**

```bash
git add binding.go registry.go
git commit -m "feat: add definitionID to useCaseConfig for variant support

Add definitionID field to useCaseConfig (empty = derive from name).
Add newUseCaseCoreWithConfig constructor for variant usecases."
```

---

### Task 2: Add NewUseCaseWithID to SDK layer

**Files:**
- Modify: `internal/sdk/usecase.go` (add NewUseCaseWithID function)

- [ ] **Step 1: Read internal/sdk/usecase.go to understand current structure**

Note: `stableUseCaseID` and `toHex` are unexported. `NewUseCase` is the current constructor.

- [ ] **Step 2: Add NewUseCaseWithID function**

In `internal/sdk/usecase.go`, add after `NewUseCase`:

```go
func NewUseCaseWithID(name string, definitionID string, kind UseCaseKind, reqs CapabilityRequirements, link RegistryLink) UseCase {
	id := definitionID
	if id == "" {
		id = stableUseCaseID(name, kind)
	}
	return useCase{
		id:           id,
		publicName:   name,
		kind:         kind,
		requirements: reqs,
		registryLink: link,
	}
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/sdk/... -v
```

Expected: All SDK tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/sdk/usecase.go
git commit -m "feat: add NewUseCaseWithID for shared definitionID support

NewUseCaseWithID accepts an optional definitionID parameter.
When empty, falls back to stableUseCaseID(name, kind) for backward compatibility."
```

---

## Chunk 2: Definition Type + Variant Methods

### Task 3: Create definition.go with DefineDefinition + helpers

**Files:**
- Create: `definition.go` (root lockman package)

- [ ] **Step 1: Create definition.go**

```go
package lockman

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
)

// Definition is a shared lock identity. Multiple variants inherit the same
// definitionID, producing the same Redis lock key for mutual exclusion.
type Definition[T any] struct {
	name    string
	id      string // stable hash: FNV-64a(name + kind)
	kind    useCaseKind
	binding Binding[T]
}

// DefineDefinition creates a shared definition that variants can inherit from.
func DefineDefinition[T any](name string, kind useCaseKind, binding Binding[T], opts ...UseCaseOption) *Definition[T] {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		panic("lockman: definition name is required")
	}
	if binding.build == nil {
		panic("lockman: binding is required")
	}
	id := stableDefinitionID(trimmed, kind)
	return &Definition[T]{name: trimmed, id: id, kind: kind, binding: binding}
}

// Variant creates a RunUseCase that inherits the definition's ID and binding.
func (d *Definition[T]) Variant(name string, opts ...UseCaseOption) RunUseCase[T] {
	if d.kind != useCaseKindRun {
		panic("lockman: Variant is only supported for Run definitions")
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		panic("lockman: variant name is required")
	}
	fullName := d.name + "." + trimmed
	cfg := useCaseConfig{definitionID: d.id}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return RunUseCase[T]{
		core:    newUseCaseCoreWithConfig(fullName, d.kind, cfg),
		binding: d.binding,
	}
}

// HoldVariant creates a HoldUseCase that inherits the definition's ID and binding.
func (d *Definition[T]) HoldVariant(name string, opts ...UseCaseOption) HoldUseCase[T] {
	if d.kind != useCaseKindHold {
		panic("lockman: HoldVariant is only supported for Hold definitions")
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		panic("lockman: variant name is required")
	}
	fullName := d.name + "." + trimmed
	cfg := useCaseConfig{definitionID: d.id}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return HoldUseCase[T]{
		core:    newUseCaseCoreWithConfig(fullName, d.kind, cfg),
		binding: d.binding,
	}
}

// ClaimVariant creates a ClaimUseCase that inherits the definition's ID and binding.
func (d *Definition[T]) ClaimVariant(name string, opts ...UseCaseOption) ClaimUseCase[T] {
	if d.kind != useCaseKindClaim {
		panic("lockman: ClaimVariant is only supported for Claim definitions")
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		panic("lockman: variant name is required")
	}
	fullName := d.name + "." + trimmed
	cfg := useCaseConfig{definitionID: d.id}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return ClaimUseCase[T]{
		core:    newUseCaseCoreWithConfig(fullName, d.kind, cfg),
		binding: d.binding,
	}
}

// Binding returns the definition's binding for use in composite member definitions.
func (d *Definition[T]) Binding() Binding[T] {
	return d.binding
}

// ForceRelease releases any lock within this definition without owner validation.
func (d *Definition[T]) ForceRelease(ctx context.Context, client *Client, resourceKey string) error {
	if client == nil {
		return fmt.Errorf("lockman: client is required")
	}
	fr, ok := client.backend.(ForceReleaseDriver)
	if !ok {
		return fmt.Errorf("lockman: backend does not support force release")
	}
	return fr.ForceReleaseDefinition(ctx, d.id, resourceKey)
}

func stableDefinitionID(name string, kind useCaseKind) string {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte{kindToByte(kind)})
	_, _ = hash.Write([]byte(name))
	return fmt.Sprintf("%016x", hash.Sum64())
}

func kindToByte(kind useCaseKind) byte {
	switch kind {
	case useCaseKindRun:
		return 'r'
	case useCaseKindClaim:
		return 'c'
	case useCaseKindHold:
		return 'h'
	default:
		return '?'
	}
}
```

- [ ] **Step 2: Compile check**

```bash
go test . -run '^$'
```

Expected: Compiles (ForceReleaseDriver not yet defined, so this will fail — that's expected for now).

- [ ] **Step 3: Commit**

```bash
git add definition.go
git commit -m "feat: add Definition[T] type with Variant/HoldVariant/ClaimVariant

Definition holds a shared definitionID. Variants inherit the ID and binding,
producing the same Redis lock key for mutual exclusion across usecases.
Add ForceRelease method for admin operations."
```

---

### Task 4: Add LineageParent function to binding.go

**Files:**
- Modify: `binding.go` (add LineageParent function)

- [ ] **Step 1: Add LineageParent function**

In `binding.go`, after the existing `UseCaseOption` functions:

```go
// LineageParent sets the lineage parent for a use case.
// The parent must be from a different definition — same-definition lineage
// is rejected at client startup with a clear error.
func LineageParent[T any](parent RunUseCase[T]) UseCaseOption {
	return func(cfg *useCaseConfig) {
		cfg.lineageParent = parent.core.name
	}
}
```

- [ ] **Step 2: Compile check**

```bash
go test . -run '^$'
```

Expected: Compiles (ForceReleaseDriver still missing — expected).

- [ ] **Step 3: Commit**

```bash
git add binding.go
git commit -m "feat: add LineageParent UseCaseOption function

LineageParent extracts the parent usecase name for lineage configuration.
Same-definition lineage (parent and child sharing definitionID) is
rejected at client startup."
```

---

## Chunk 3: Client Validation + SDK Integration

### Task 5: Update normalizeUseCase to pass definitionID

**Files:**
- Modify: `client_validation.go:160-171` (normalizeUseCase function)

- [ ] **Step 1: Update normalizeUseCase**

In `client_validation.go`, change `normalizeUseCase` to use `NewUseCaseWithID`:

```go
func normalizeUseCase(useCase *useCaseCore, childCounts map[string]int, link sdk.RegistryLink) sdk.UseCase {
	return sdk.NewUseCaseWithID(
		useCase.name,
		useCase.config.definitionID,
		toSDKUseCaseKind(useCase.kind),
		sdk.CapabilityRequirements{
			RequiresIdempotency: useCase.kind == useCaseKindClaim && useCase.config.idempotent,
			RequiresStrict:      useCase.config.strict,
			RequiresLineage:     strings.TrimSpace(useCase.config.lineageParent) != "" || childCounts[useCase.name] > 0,
		},
		link,
	)
}
```

- [ ] **Step 2: Compile check**

```bash
go test . -run '^$'
```

Expected: Compiles (ForceReleaseDriver still missing).

- [ ] **Step 3: Commit**

```bash
git add client_validation.go
git commit -m "feat: pass definitionID through normalizeUseCase to SDK

Use NewUseCaseWithID instead of NewUseCase. When definitionID is empty,
SDK falls back to deriving ID from name (backward compatible)."
```

---

### Task 6: Add same-definition lineage enforcement

**Files:**
- Modify: `client_validation.go:26-103` (buildClientPlan function)

- [ ] **Step 1: Add same-definition lineage check in buildClientPlan**

In `buildClientPlan`, after the `normalizedByName` loop and before `sdk.ValidateCapabilities`, add:

```go
// Reject same-definition lineage (parent and child sharing definitionID)
for _, useCase := range useCases {
	parentName := strings.TrimSpace(useCase.config.lineageParent)
	if parentName == "" {
		continue
	}
	parent, ok := cfg.registry.byName[parentName]
	if !ok {
		continue // caught by existing lineage parent validation
	}
	if useCase.config.definitionID != "" &&
		useCase.config.definitionID == parent.config.definitionID {
		return clientPlan{}, fmt.Errorf(
			"lockman: use case %q cannot use lineage parent %q — both share the same definition",
			useCase.name, parentName)
	}
}
```

- [ ] **Step 2: Run tests**

```bash
go test . -v -run 'TestBuildClientPlan'
```

Expected: Existing tests pass.

- [ ] **Step 3: Commit**

```bash
git add client_validation.go
git commit -m "feat: reject same-definition lineage at client startup

When parent and child usecases share the same definitionID, lineage
resolution would cause self-reference (child's ParentRef == child's ID).
Reject with clear error message."
```

---

## Chunk 4: Backend Changes

### Task 7: Add ForceReleaseDriver interface + Redis implementation

**Files:**
- Modify: `backend/contracts.go` (add ForceReleaseDriver interface)
- Modify: `backend/redis/driver.go` (implement ForceReleaseDefinition)

- [ ] **Step 1: Add ForceReleaseDriver to backend/contracts.go**

After the existing `LineageDriver` interface:

```go
// ForceReleaseDriver is an optional interface for force-releasing locks
// without owner validation. Used by Definition.ForceRelease for admin operations.
type ForceReleaseDriver interface {
	ForceReleaseDefinition(ctx context.Context, definitionID, resourceKey string) error
}
```

- [ ] **Step 2: Implement ForceReleaseDefinition in backend/redis/driver.go**

Add to `driver.go`:

```go
func (d *Driver) ForceReleaseDefinition(ctx context.Context, definitionID, resourceKey string) error {
	keys := []string{
		d.buildLeaseKey(definitionID, resourceKey),
		d.buildStrictFenceCounterKey(definitionID, resourceKey),
		d.buildStrictTokenKey(definitionID, resourceKey),
		d.buildLineageKey(definitionID, resourceKey),
	}
	if err := d.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("lockman: force release definition: %w", err)
	}
	return nil
}
```

- [ ] **Step 3: Run full test suite**

```bash
go test ./... -v
```

Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add backend/contracts.go backend/redis/driver.go
git commit -m "feat: add ForceReleaseDriver optional interface and Redis implementation

ForceReleaseDriver follows the existing StrictDriver/LineageDriver pattern.
ForceReleaseDefinition deletes all possible auxiliary keys (lease, fence,
token, lineage) unconditionally — DEL on non-existent keys is a no-op."
```

---

## Chunk 5: Tests + Example

### Task 8: Write definition_test.go

**Files:**
- Create: `definition_test.go` (root lockman package)

- [ ] **Step 1: Create definition_test.go with unit tests**

```go
package lockman

import (
	"testing"
	"time"
)

func TestDefineDefinitionPanicsWithoutName(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for empty name")
		}
	}()
	DefineDefinition[string]("", useCaseKindRun, Binding[string]{})
}

func TestDefineDefinitionPanicsWithoutBinding(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil binding")
		}
	}()
	DefineDefinition[string]("test", useCaseKindRun, Binding[string]{})
}

func TestDefineDefinitionCreatesStableID(t *testing.T) {
	def1 := DefineDefinition("order", useCaseKindRun, BindResourceID("order", func(o string) string { return o }))
	def2 := DefineDefinition("order", useCaseKindRun, BindResourceID("order", func(o string) string { return o }))
	if def1.id != def2.id {
		t.Fatalf("expected same ID for same name+kind, got %q vs %q", def1.id, def2.id)
	}
}

func TestVariantInheritsDefinitionID(t *testing.T) {
	def := DefineDefinition("contract", useCaseKindRun, BindResourceID("order", func(o string) string { return o }))
	uc := def.Variant("import")
	if uc.core.config.definitionID != def.id {
		t.Fatalf("expected variant to inherit definitionID %q, got %q", def.id, uc.core.config.definitionID)
	}
}

func TestVariantNameFormat(t *testing.T) {
	def := DefineDefinition("contract", useCaseKindRun, BindResourceID("order", func(o string) string { return o }))
	uc := def.Variant("import")
	if uc.core.name != "contract.import" {
		t.Fatalf("expected name %q, got %q", "contract.import", uc.core.name)
	}
}

func TestVariantPanicsWithEmptyName(t *testing.T) {
	def := DefineDefinition("contract", useCaseKindRun, BindResourceID("order", func(o string) string { return o }))
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for empty variant name")
		}
	}()
	def.Variant("")
}

func TestVariantPanicsForWrongKind(t *testing.T) {
	def := DefineDefinition("contract", useCaseKindHold, BindResourceID("order", func(o string) string { return o }))
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for Variant on Hold definition")
		}
	}()
	def.Variant("import")
}

func TestHoldVariantInheritsDefinitionID(t *testing.T) {
	def := DefineDefinition("contract", useCaseKindHold, BindResourceID("order", func(o string) string { return o }))
	uc := def.HoldVariant("hold")
	if uc.core.config.definitionID != def.id {
		t.Fatalf("expected HoldVariant to inherit definitionID %q, got %q", def.id, uc.core.config.definitionID)
	}
}

func TestClaimVariantInheritsDefinitionID(t *testing.T) {
	def := DefineDefinition("contract", useCaseKindClaim, BindResourceID("order", func(o string) string { return o }))
	uc := def.ClaimVariant("claim")
	if uc.core.config.definitionID != def.id {
		t.Fatalf("expected ClaimVariant to inherit definitionID %q, got %q", def.id, uc.core.config.definitionID)
	}
}

func TestDefinitionBindingReturnsBinding(t *testing.T) {
	binding := BindResourceID("order", func(o string) string { return o })
	def := DefineDefinition("contract", useCaseKindRun, binding)
	if def.Binding().build == nil {
		t.Fatal("expected Binding() to return non-nil binding")
	}
}

func TestStableDefinitionIDDifferentKinds(t *testing.T) {
	binding := BindResourceID("order", func(o string) string { return o })
	runID := DefineDefinition("order", useCaseKindRun, binding).id
	holdID := DefineDefinition("order", useCaseKindHold, binding).id
	claimID := DefineDefinition("order", useCaseKindClaim, binding).id
	if runID == holdID || runID == claimID || holdID == claimID {
		t.Fatalf("expected different IDs for different kinds: run=%q hold=%q claim=%q", runID, holdID, claimID)
	}
}
```

- [ ] **Step 2: Run definition tests**

```bash
go test . -run 'TestDefine|TestVariant|TestHold|TestClaim|TestDefinition|TestStable' -v
```

Expected: All tests pass.

- [ ] **Step 3: Commit**

```bash
git add definition_test.go
git commit -m "test: add unit tests for Definition + Variant API

Test DefineDefinition panics, stable ID generation, Variant/HoldVariant/
ClaimVariant inheritance, name format, kind guards, and Binding accessor."
```

---

### Task 9: Add same-definition lineage rejection test

**Files:**
- Modify: `client_test.go` or `client_validation_test.go` (add test for same-definition lineage)

- [ ] **Step 1: Find existing lineage tests**

```bash
grep -n "lineageParent" client_test.go
```

- [ ] **Step 2: Add same-definition lineage rejection test**

Add to `client_test.go`:

```go
func TestSameDefinitionLineageRejected(t *testing.T) {
	// Create a definition with two variants
	def := DefineDefinition("contract", useCaseKindRun, BindResourceID("order", func(o string) string { return o }))
	parentUC := def.Variant("parent")
	childUC := def.Variant("child", LineageParent(parentUC))

	reg := NewRegistry()
	if err := reg.Register(parentUC, childUC); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err := NewClient(
		WithRegistry(reg),
		WithBackend(newTestBackend(t)),
		WithIdentity(Identity{OwnerID: "test"}),
	)
	if err == nil {
		t.Fatal("expected error for same-definition lineage")
	}
	if !strings.Contains(err.Error(), "both share the same definition") {
		t.Fatalf("expected same-definition lineage error, got: %v", err)
	}
}
```

- [ ] **Step 3: Run the test**

```bash
go test . -run 'TestSameDefinitionLineageRejected' -v
```

Expected: Test passes.

- [ ] **Step 4: Commit**

```bash
git add client_test.go
git commit -m "test: add same-definition lineage rejection test

Verify that buildClientPlan rejects lineage between variants of the
same definition, preventing self-reference in ParentRef."
```

---

### Task 10: Add integration test for variant mutual exclusion

**Files:**
- Modify: `client_test.go` (add integration test)

- [ ] **Step 1: Add variant mutual exclusion test**

Add to `client_test.go`:

```go
func TestVariantMutualExclusion(t *testing.T) {
	// Two variants of the same definition should contend on the same lock
	def := DefineDefinition("contract", useCaseKindRun, BindResourceID("order", func(o string) string { return o }))
	importUC := def.Variant("import")
	deleteUC := def.Variant("delete")

	reg := NewRegistry()
	if err := reg.Register(importUC, deleteUC); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	client, err := NewClient(
		WithRegistry(reg),
		WithBackend(newTestBackend(t)),
		WithIdentity(Identity{OwnerID: "test"}),
	)
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}

	ctx := context.Background()

	// importUC acquires the lock
	importReq, err := importUC.With("order:123")
	if err != nil {
		t.Fatalf("importUC.With failed: %v", err)
	}

	var importDone chan struct{}
	importErr := make(chan error, 1)
	importDone = make(chan struct{})
	go func() {
		importErr <- client.Run(ctx, importReq, func(ctx context.Context, lease LeaseContext) error {
			close(importDone)
			time.Sleep(100 * time.Millisecond)
			return nil
		})
	}()

	// Wait for importUC to acquire
	<-importDone

	// deleteUC should fail to acquire (same resource, same definitionID)
	deleteReq, err := deleteUC.With("order:123")
	if err != nil {
		t.Fatalf("deleteUC.With failed: %v", err)
	}

	err = client.Run(ctx, deleteReq, func(ctx context.Context, lease LeaseContext) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected deleteUC to fail acquiring lock held by importUC")
	}
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy, got: %v", err)
	}
}
```

- [ ] **Step 2: Run the test**

```bash
go test . -run 'TestVariantMutualExclusion' -v
```

Expected: Test passes.

- [ ] **Step 3: Commit**

```bash
git add client_test.go
git commit -m "test: add variant mutual exclusion integration test

Verify that two variants of the same definition contend on the same
Redis lock key — when one holds the lock, the other receives ErrBusy."
```

---

### Task 11: Create example usage

**Files:**
- Create: `examples/core/definition-variant/main.go`

- [ ] **Step 1: Create example**

```go
// Command definition-variant demonstrates the Definition + Variant API
// for shared lock keys across multiple usecases.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/backend/redis"
)

type OrderInput struct {
	OrderID string
}

func main() {
	// Shared definition — all variants use the same lock key
	contract := lockman.DefineDefinition("contract", lockman.Run,
		lockman.BindResourceID("order", func(i OrderInput) string { return i.OrderID }),
		lockman.TTL(30*time.Second),
	)

	// Variants inherit the shared definitionID
	importUC := contract.Variant("import")
	deleteUC := contract.Variant("delete")

	// Register usecases
	reg := lockman.NewRegistry()
	if err := reg.Register(importUC, deleteUC); err != nil {
		log.Fatal(err)
	}

	// Create client
	rdb := redis.NewClient(...) // configure your Redis
	backend, err := redis.NewBackend(rdb)
	if err != nil {
		log.Fatal(err)
	}

	client, err := lockman.NewClient(
		lockman.WithRegistry(reg),
		lockman.WithBackend(backend),
		lockman.WithIdentity(lockman.Identity{OwnerID: "service-1"}),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// importUC acquires the lock
	req, err := importUC.With(OrderInput{OrderID: "123"})
	if err != nil {
		log.Fatal(err)
	}

	if err := client.Run(ctx, req, func(ctx context.Context, lease lockman.LeaseContext) error {
		fmt.Printf("importUC acquired lock on order:123 (definitionID: %s)\n", lease.DefinitionID)
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	// deleteUC will contend on the same lock
	req, err = deleteUC.With(OrderInput{OrderID: "123"})
	if err != nil {
		log.Fatal(err)
	}

	err = client.Run(ctx, req, func(ctx context.Context, lease lockman.LeaseContext) error {
		fmt.Printf("deleteUC acquired lock on order:123 (definitionID: %s)\n", lease.DefinitionID)
		return nil
	})
	if err != nil {
		fmt.Printf("deleteUC failed (expected if importUC still holds lock): %v\n", err)
	}
}
```

- [ ] **Step 2: Verify example compiles**

```bash
go test -tags lockman_examples ./examples/core/definition-variant/... -run '^$'
```

Expected: Compiles.

- [ ] **Step 3: Commit**

```bash
git add examples/core/definition-variant/main.go
git commit -m "docs: add definition-variant example

Demonstrates DefineDefinition, Variant, and mutual exclusion between
variants sharing the same lock key."
```

---

## Chunk 6: Verification + Cleanup

### Task 12: Run full test suite + CI parity checks

- [ ] **Step 1: Run full workspace test suite**

```bash
go test ./...
```

- [ ] **Step 2: Run without workspace mode**

```bash
GOWORK=off go test ./...
```

- [ ] **Step 3: Run module-specific tests**

```bash
go test ./backend/redis/...
go test ./idempotency/redis/...
go test ./guard/postgres/...
```

- [ ] **Step 4: Compile examples**

```bash
go test -tags lockman_examples ./examples/... -run '^$'
```

- [ ] **Step 5: Run lint**

```bash
make lint
```

- [ ] **Step 6: Run tidy**

```bash
make tidy
```

- [ ] **Step 7: Commit any formatting changes**

```bash
git add -A
git commit -m "chore: apply gofmt and dependency updates"
```

---

### Task 13: Verify backward compatibility

- [ ] **Step 1: Verify existing tests still pass**

```bash
go test ./... -v 2>&1 | grep -E '(PASS|FAIL|---)'
```

Expected: All existing tests pass. No regressions.

- [ ] **Step 2: Verify DefineRun unchanged**

Check that existing code using `DefineRun` still compiles and works:

```bash
grep -r "DefineRun" examples/ | head -5
```

Verify those examples still compile:

```bash
go test -tags lockman_examples ./examples/... -run '^$'
```

- [ ] **Step 3: Final commit**

```bash
git add -A
git commit -m "chore: verify backward compatibility

All existing tests pass. DefineRun/Hold/Claim unchanged.
Definition + Variant is additive API."
```

---

## Summary

| Chunk | Tasks | Files | Estimated Time |
|-------|-------|-------|----------------|
| 1 | Task 1-2 | `binding.go`, `registry.go`, `internal/sdk/usecase.go` | 15 min |
| 2 | Task 3-4 | `definition.go`, `binding.go` | 20 min |
| 3 | Task 5-6 | `client_validation.go` | 15 min |
| 4 | Task 7 | `backend/contracts.go`, `backend/redis/driver.go` | 15 min |
| 5 | Task 8-11 | `definition_test.go`, `client_test.go`, `examples/` | 30 min |
| 6 | Task 12-13 | Verification | 15 min |

**Total: ~110 minutes**
