# Multiple Lock (RunMultiple / HoldMultiple) Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `RunMultiple` and `HoldMultiple` client methods that acquire multiple keys of the same definition in one atomic all-or-nothing operation.

**Architecture:** New `ExecuteMultipleExclusive` in the runtime engine (mirrors composite pattern but for single-definition multi-key). Client methods `RunMultiple` and `HoldMultiple` delegate to it. HoldMultiple encodes all keys into a single hold token via existing `sdk.EncodeHoldToken`. No backend changes, no registry changes.

**Tech Stack:** Go 1.22, lockman internal SDK, Redis backend for examples

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `lockkit/definitions/ownership.go` | Modify | Add `MultipleLockRequest` type |
| `lockkit/runtime/multiple.go` | Create | `ExecuteMultipleExclusive` engine method |
| `lockkit/runtime/multiple_test.go` | Create | Engine-level unit tests |
| `client_multiple.go` | Create | `RunMultiple`, `HoldMultiple` public methods |
| `client_multiple_test.go` | Create | Client-level unit tests |
| `examples/sdk/multiple-run/main.go` | Create | RunMultiple example |
| `examples/sdk/multiple-hold/main.go` | Create | HoldMultiple example |
| `docs/multiple-lock.md` | Create | User-facing documentation |

---

## Chunk 1: Engine Layer — MultipleLockRequest + ExecuteMultipleExclusive

### Task 1: Add MultipleLockRequest type

**Files:**
- Modify: `lockkit/definitions/ownership.go`

- [ ] **Step 1: Add MultipleLockRequest type**

Append to `lockkit/definitions/ownership.go` after `CompositeClaimRequest`:

```go
// MultipleLockRequest is the payload for synchronous multiple-key acquire attempts.
type MultipleLockRequest struct {
	DefinitionID string
	Keys         []string
	Ownership    OwnershipMeta
	Overrides    *RuntimeOverrides
}
```

- [ ] **Step 2: Run tests to verify no breakage**

Run: `go test ./lockkit/definitions/... -v`
Expected: PASS

---

### Task 2: Write ExecuteMultipleExclusive tests (TDD — fail first)

**Files:**
- Create: `lockkit/runtime/multiple_test.go`
- Reference: `lockkit/runtime/composite_test.go` (if exists), `lockkit/runtime/exclusive_test.go`

- [ ] **Step 1: Write failing tests for ExecuteMultipleExclusive**

Create `lockkit/runtime/multiple_test.go` with these test cases:

```go
package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/registry"
)

func TestExecuteMultipleExclusiveAcquiresAllKeys(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:         "order",
		Kind:       definitions.KindParent,
		Resource:   "order",
		Mode:       definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:   5 * time.Second,
		KeyBuilder: definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	drv := &mockMultipleDriver{}
	mgr := newTestMultipleManager(t, reg, drv)

	called := false
	var gotKeys []string
	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{"order:1", "order:2", "order:3"},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		called = true
		gotKeys = append([]string(nil), lc.ResourceKeys...)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("callback was not called")
	}
	if len(gotKeys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(gotKeys))
	}
}

func TestExecuteMultipleExclusiveFailsOnBusy(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:         "order",
		Kind:       definitions.KindParent,
		Resource:   "order",
		Mode:       definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:   5 * time.Second,
		KeyBuilder: definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	drv := &mockMultipleDriver{failOnKey: "order:2"}
	mgr := newTestMultipleManager(t, reg, drv)

	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{"order:1", "order:2", "order:3"},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		t.Fatal("callback should not be called on failure")
		return nil
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, lockerrors.ErrLockBusy) {
		t.Fatalf("expected ErrLockBusy, got: %v", err)
	}
	if drv.releaseCount != 1 {
		t.Fatalf("expected 1 release (for order:1 rollback), got %d", drv.releaseCount)
	}
}

func TestExecuteMultipleExclusiveRejectsEmptyKeys(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:         "order",
		Kind:       definitions.KindParent,
		Resource:   "order",
		Mode:       definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:   5 * time.Second,
		KeyBuilder: definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	mgr := newTestMultipleManager(t, reg, &mockMultipleDriver{})

	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error for empty keys")
	}
}

func TestExecuteMultipleExclusiveRejectsDuplicateKeys(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:         "order",
		Kind:       definitions.KindParent,
		Resource:   "order",
		Mode:       definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:   5 * time.Second,
		KeyBuilder: definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	mgr := newTestMultipleManager(t, reg, &mockMultipleDriver{})

	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{"order:1", "order:1"},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error for duplicate keys")
	}
}

func TestExecuteMultipleExclusiveRejectsShuttingDown(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:         "order",
		Kind:       definitions.KindParent,
		Resource:   "order",
		Mode:       definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:   5 * time.Second,
		KeyBuilder: definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	mgr := newTestMultipleManager(t, reg, &mockMultipleDriver{})
	mgr.Shutdown(context.Background())

	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{"order:1"},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error during shutdown")
	}
}

func TestExecuteMultipleExclusiveCanonicalOrder(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:         "order",
		Kind:       definitions.KindParent,
		Resource:   "order",
		Mode:       definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:   5 * time.Second,
		KeyBuilder: definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	drv := &mockMultipleDriver{captureOrder: true}
	mgr := newTestMultipleManager(t, reg, drv)

	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{"order:3", "order:1", "order:2"},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(drv.acquireOrder) != 3 {
		t.Fatalf("expected 3 acquires, got %d", len(drv.acquireOrder))
	}
	expected := []string{"order:1", "order:2", "order:3"}
	for i, want := range expected {
		if drv.acquireOrder[i] != want {
			t.Errorf("acquire[%d] = %q, want %q", i, drv.acquireOrder[i], want)
		}
	}
}

func TestExecuteMultipleExclusiveRejectsStrictDefinition(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:         "order",
		Kind:       definitions.KindParent,
		Resource:   "order",
		Mode:       definitions.ModeStrict,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:   5 * time.Second,
		KeyBuilder: definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	mgr := newTestMultipleManager(t, reg, &mockMultipleDriver{})

	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{"order:1"},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error for strict definition")
	}
}

// mockMultipleDriver is a test double for backend.Driver
type mockMultipleDriver struct {
	failOnKey    string
	captureOrder bool
	acquireOrder []string
	releaseCount int
}

func (d *mockMultipleDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	if d.captureOrder {
		d.acquireOrder = append(d.acquireOrder, req.ResourceKeys[0])
	}
	if req.ResourceKeys[0] == d.failOnKey {
		return backend.LeaseRecord{}, backend.ErrLeaseAlreadyHeld
	}
	return backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: req.ResourceKeys,
		OwnerID:      req.OwnerID,
		AcquiredAt:   time.Now(),
		ExpiresAt:    time.Now().Add(req.LeaseTTL),
		LeaseTTL:     req.LeaseTTL,
	}, nil
}

func (d *mockMultipleDriver) Renew(ctx context.Context, rec backend.LeaseRecord) (backend.LeaseRecord, error) {
	return rec, nil
}

func (d *mockMultipleDriver) Release(ctx context.Context, rec backend.LeaseRecord) error {
	d.releaseCount++
	return nil
}

func (d *mockMultipleDriver) CheckPresence(ctx context.Context, definitionID, resourceKey string) (definitions.PresenceStatus, error) {
	return definitions.PresenceStatus{State: definitions.PresenceNotHeld}, nil
}

func (d *mockMultipleDriver) Ping(ctx context.Context) error {
	return nil
}

func newTestMultipleManager(t *testing.T, reg *registry.Registry, drv backend.Driver) *Manager {
	t.Helper()
	mgr, err := NewManager(reg, drv)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	return mgr
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./lockkit/runtime/ -run 'TestExecuteMultiple' -v`
Expected: FAIL — `ExecuteMultipleExclusive` does not exist yet

---

### Task 3: Implement ExecuteMultipleExclusive

**Files:**
- Create: `lockkit/runtime/multiple.go`
- Reference: `lockkit/runtime/composite.go`, `lockkit/runtime/exclusive.go`

- [ ] **Step 1: Create lockkit/runtime/multiple.go**

```go
package runtime

import (
	"context"
	stdErrors "errors"
	"sort"
	"time"

	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/observe"
)

type acquiredMultipleLease struct {
	resourceKey string
	held        heldLease
}

// ExecuteMultipleExclusive runs fn after acquiring multiple keys of the same definition in canonical order.
// All keys must be acquired successfully (all-or-nothing). If any key fails, all previously acquired keys are released.
func (m *Manager) ExecuteMultipleExclusive(
	ctx context.Context,
	req definitions.MultipleLockRequest,
	fn func(context.Context, definitions.LeaseContext) error,
) (retErr error) {
	if m.isShuttingDown() {
		return lockerrors.ErrPolicyViolation
	}

	def, ok := m.getDefinition(req.DefinitionID)
	if !ok {
		return lockerrors.ErrPolicyViolation
	}
	if def.Mode == definitions.ModeStrict {
		return lockerrors.ErrPolicyViolation
	}
	if len(req.Keys) == 0 {
		return lockerrors.ErrPolicyViolation
	}
	if hasDuplicateKeys(req.Keys) {
		return lockerrors.ErrPolicyViolation
	}

	keys := make([]string, len(req.Keys))
	copy(keys, req.Keys)
	sort.Strings(keys)

	if !m.tryAdmitInFlightExecution() {
		return lockerrors.ErrPolicyViolation
	}
	admitted := true
	defer func() {
		if admitted {
			m.releaseInFlightExecution()
		}
	}()

	guardKeys := make([]guardKey, len(keys))
	for i, key := range keys {
		guardKeys[i] = guardKey{
			definitionID: def.ID,
			resourceKey:  key,
			ownerID:      req.Ownership.OwnerID,
		}
		if _, loaded := m.active.LoadOrStore(guardKeys[i], guardEntry{state: guardPending}); loaded {
			return lockerrors.ErrReentrantAcquire
		}
	}
	guardInstalled := true
	defer func() {
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
	}()

	waitConfig, err := applyRuntimeOverrides(def, req.Overrides)
	if err != nil {
		return err
	}

	acquired := make([]acquiredMultipleLease, 0, len(keys))
	defer func() {
		for i := len(acquired) - 1; i >= 0; i-- {
			lease := acquired[i]
			held := time.Since(lease.held.lease.AcquiredAt)
			m.recorder.RecordRelease(ctx, def.ID, held)
			if m.bridge != nil {
				m.bridge.PublishRuntimeReleased(observe.Event{
					Kind:         observe.EventReleased,
					DefinitionID: def.ID,
					ResourceID:   lease.resourceKey,
					OwnerID:      req.Ownership.OwnerID,
					RequestID:    req.Ownership.RequestID,
					Held:         held,
				})
			}
			if releaseErr := m.releaseLease(context.Background(), lease.held); releaseErr != nil {
				if retErr == nil {
					retErr = releaseErr
				} else {
					retErr = stdErrors.Join(retErr, releaseErr)
				}
			}
		}
	}()

	for i, key := range keys {
		acquireCtx, cancel := contextWithAcquireTimeout(ctx, waitConfig)
		re := observe.Event{
			Kind:         observe.EventAcquireStarted,
			DefinitionID: def.ID,
			ResourceID:   key,
			OwnerID:      req.Ownership.OwnerID,
			RequestID:    req.Ownership.RequestID,
		}
		if m.bridge != nil {
			m.bridge.PublishRuntimeAcquireStarted(re)
		}
		start := time.Now()
		lease, acquireErr := m.acquireLease(acquireCtx, def, runtimeAcquirePlan{resourceKey: key}, req.Ownership.OwnerID)
		waitDuration := time.Since(start)
		cancel()

		re.Wait = waitDuration
		m.recorder.RecordAcquire(ctx, def.ID, waitDuration, acquireErr == nil)
		if acquireErr != nil {
			recordAcquireFailure(m, ctx, def.ID, acquireErr)
			if m.bridge != nil {
				m.bridge.PublishRuntimeAcquireFailed(re, acquireErr)
				recordBridgeAcquireFailure(m, re, acquireErr)
			}
			return mapAcquireError(acquireErr)
		}

		if m.bridge != nil {
			m.bridge.PublishRuntimeAcquireSucceeded(re)
		}

		acquired = append(acquired, acquiredMultipleLease{
			resourceKey: key,
			held:        lease,
		})
		m.active.Store(guardKeys[i], guardEntry{state: guardHeld})
		m.activeCounter(def.ID).Add(1)
		m.recordActiveLocks(ctx, def.ID)
	}

	retErr = fn(ctx, buildMultipleLeaseContext(req, acquired))
	return retErr
}

func hasDuplicateKeys(keys []string) bool {
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			return true
		}
		seen[key] = struct{}{}
	}
	return false
}

func buildMultipleLeaseContext(req definitions.MultipleLockRequest, acquired []acquiredMultipleLease) definitions.LeaseContext {
	resourceKeys := make([]string, len(acquired))
	var minTTL time.Duration
	var leaseDeadline time.Time

	for i, lease := range acquired {
		resourceKeys[i] = lease.resourceKey
		if i == 0 || lease.held.lease.LeaseTTL < minTTL {
			minTTL = lease.held.lease.LeaseTTL
		}
		if i == 0 || lease.held.lease.ExpiresAt.Before(leaseDeadline) {
			leaseDeadline = lease.held.lease.ExpiresAt
		}
	}

	return definitions.LeaseContext{
		DefinitionID:  req.DefinitionID,
		ResourceKeys:  resourceKeys,
		Ownership:     req.Ownership,
		LeaseTTL:      minTTL,
		LeaseDeadline: leaseDeadline,
	}
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./lockkit/runtime/ -run 'TestExecuteMultiple' -v`
Expected: All 7 tests PASS

- [ ] **Step 3: Run full runtime tests**

Run: `go test ./lockkit/runtime/... -v`
Expected: All tests PASS (existing + new)

- [ ] **Step 4: Commit**

```bash
git add lockkit/definitions/ownership.go lockkit/runtime/multiple.go lockkit/runtime/multiple_test.go
git commit -m "feat: add ExecuteMultipleExclusive engine for multi-key same-definition acquire"
```

---

## Chunk 2: Client Layer — RunMultiple + HoldMultiple

### Task 4: Write RunMultiple client tests (TDD — fail first)

**Files:**
- Create: `client_multiple_test.go`
- Reference: `client_run_test.go`, `client_hold_test.go`

- [ ] **Step 1: Write failing tests for RunMultiple and HoldMultiple**

Create `client_multiple_test.go`:

```go
package lockman

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/idempotency"
	"github.com/tuanuet/lockman/lockkit/definitions"
)

type batchOrderInput struct {
	OrderID string
}

func TestRunMultipleAcquiresAllKeys(t *testing.T) {
	orderDef := DefineLock(
		"order",
		BindResourceID("order", func(in batchOrderInput) string { return in.OrderID }),
	)
	batchUC := DefineRunOn("batch_process", orderDef, TTL(5*time.Second))

	reg := NewRegistry()
	if err := reg.Register(batchUC); err != nil {
		t.Fatal(err)
	}

	drv := &trackingMultipleBackend{}
	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(drv),
		WithIdempotency(idempotency.NewMemoryStore()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	var gotKeys []string
	err = client.RunMultiple(context.Background(), batchUC, func(ctx context.Context, lease Lease) error {
		gotKeys = append([]string(nil), lease.ResourceKeys...)
		return nil
	}, batchOrderInput{}, []string{"order:1", "order:2", "order:3"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(gotKeys) != 3 {
		t.Fatalf("expected 3 keys, got %d: %v", len(gotKeys), gotKeys)
	}
}

func TestRunMultipleAllOrNothing(t *testing.T) {
	orderDef := DefineLock(
		"order",
		BindResourceID("order", func(in batchOrderInput) string { return in.OrderID }),
	)
	batchUC := DefineRunOn("batch_process", orderDef, TTL(5*time.Second))

	reg := NewRegistry()
	if err := reg.Register(batchUC); err != nil {
		t.Fatal(err)
	}

	drv := &trackingMultipleBackend{failOnKey: "order:2"}
	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(drv),
		WithIdempotency(idempotency.NewMemoryStore()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	called := false
	err = client.RunMultiple(context.Background(), batchUC, func(ctx context.Context, lease Lease) error {
		called = true
		return nil
	}, batchOrderInput{}, []string{"order:1", "order:2", "order:3"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy, got: %v", err)
	}
	if called {
		t.Fatal("callback should not be called on failure")
	}
}

func TestRunMultipleRejectsEmptyKeys(t *testing.T) {
	orderDef := DefineLock(
		"order",
		BindResourceID("order", func(in batchOrderInput) string { return in.OrderID }),
	)
	batchUC := DefineRunOn("batch_process", orderDef)

	reg := NewRegistry()
	if err := reg.Register(batchUC); err != nil {
		t.Fatal(err)
	}

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(&trackingMultipleBackend{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	err = client.RunMultiple(context.Background(), batchUC, func(ctx context.Context, lease Lease) error {
		return nil
	}, batchOrderInput{}, []string{})

	if err == nil {
		t.Fatal("expected error for empty keys")
	}
}

func TestRunMultipleRejectsDuplicateKeys(t *testing.T) {
	orderDef := DefineLock(
		"order",
		BindResourceID("order", func(in batchOrderInput) string { return in.OrderID }),
	)
	batchUC := DefineRunOn("batch_process", orderDef)

	reg := NewRegistry()
	if err := reg.Register(batchUC); err != nil {
		t.Fatal(err)
	}

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(&trackingMultipleBackend{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	err = client.RunMultiple(context.Background(), batchUC, func(ctx context.Context, lease Lease) error {
		return nil
	}, batchOrderInput{}, []string{"order:1", "order:1"})

	if err == nil {
		t.Fatal("expected error for duplicate keys")
	}
}

func TestRunMultipleRejectsNilCallback(t *testing.T) {
	orderDef := DefineLock(
		"order",
		BindResourceID("order", func(in batchOrderInput) string { return in.OrderID }),
	)
	batchUC := DefineRunOn("batch_process", orderDef)

	reg := NewRegistry()
	if err := reg.Register(batchUC); err != nil {
		t.Fatal(err)
	}

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(&trackingMultipleBackend{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	err = client.RunMultiple(context.Background(), batchUC, nil, batchOrderInput{}, []string{"order:1"})

	if err == nil {
		t.Fatal("expected error for nil callback")
	}
}

func TestHoldMultipleAcquiresAllKeys(t *testing.T) {
	slotDef := DefineLock(
		"slot",
		BindResourceID("slot", func(in batchOrderInput) string { return in.OrderID }),
	)
	holdUC := DefineHoldOn("hold_slots", slotDef, TTL(5*time.Second))

	reg := NewRegistry()
	if err := reg.Register(holdUC); err != nil {
		t.Fatal(err)
	}

	drv := &trackingMultipleBackend{}
	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(drv),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	handle, err := client.HoldMultiple(context.Background(), holdUC, batchOrderInput{}, []string{"slot:1", "slot:2", "slot:3"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handle.Token() == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestHoldMultipleForfeitReleasesAllKeys(t *testing.T) {
	slotDef := DefineLock(
		"slot",
		BindResourceID("slot", func(in batchOrderInput) string { return in.OrderID }),
	)
	holdUC := DefineHoldOn("hold_slots", slotDef, TTL(5*time.Second))

	reg := NewRegistry()
	if err := reg.Register(holdUC); err != nil {
		t.Fatal(err)
	}

	drv := &trackingMultipleBackend{}
	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(drv),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	handle, err := client.HoldMultiple(context.Background(), holdUC, batchOrderInput{}, []string{"slot:1", "slot:2"})
	if err != nil {
		t.Fatal(err)
	}

	err = client.Forfeit(context.Background(), holdUC.ForfeitWith(handle.Token()))
	if err != nil {
		t.Fatalf("unexpected forfeit error: %v", err)
	}
}

func TestHoldMultipleRejectsEmptyKeys(t *testing.T) {
	slotDef := DefineLock(
		"slot",
		BindResourceID("slot", func(in batchOrderInput) string { return in.OrderID }),
	)
	holdUC := DefineHoldOn("hold_slots", slotDef)

	reg := NewRegistry()
	if err := reg.Register(holdUC); err != nil {
		t.Fatal(err)
	}

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "test-worker"}),
		WithBackend(&trackingMultipleBackend{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	_, err = client.HoldMultiple(context.Background(), holdUC, batchOrderInput{}, []string{})

	if err == nil {
		t.Fatal("expected error for empty keys")
	}
}

// trackingMultipleBackend tracks acquire/release calls for test assertions
type trackingMultipleBackend struct {
	failOnKey   string
	acquireKeys []string
	releaseKeys []string
}

func (d *trackingMultipleBackend) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	key := req.ResourceKeys[0]
	d.acquireKeys = append(d.acquireKeys, key)
	if key == d.failOnKey {
		return backend.LeaseRecord{}, backend.ErrLeaseAlreadyHeld
	}
	return backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: req.ResourceKeys,
		OwnerID:      req.OwnerID,
		AcquiredAt:   time.Now(),
		ExpiresAt:    time.Now().Add(req.LeaseTTL),
		LeaseTTL:     req.LeaseTTL,
	}, nil
}

func (d *trackingMultipleBackend) Renew(ctx context.Context, rec backend.LeaseRecord) (backend.LeaseRecord, error) {
	return rec, nil
}

func (d *trackingMultipleBackend) Release(ctx context.Context, rec backend.LeaseRecord) error {
	d.releaseKeys = append(d.releaseKeys, rec.ResourceKeys...)
	return nil
}

func (d *trackingMultipleBackend) CheckPresence(ctx context.Context, definitionID, resourceKey string) (definitions.PresenceStatus, error) {
	return definitions.PresenceStatus{State: definitions.PresenceNotHeld}, nil
}

func (d *trackingMultipleBackend) Ping(ctx context.Context) error {
	return nil
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestRunMultiple|TestHoldMultiple' -v`
Expected: FAIL — `RunMultiple` and `HoldMultiple` do not exist

---

### Task 5: Implement RunMultiple and HoldMultiple

**Files:**
- Create: `client_multiple.go`
- Reference: `client_run.go`, `client_hold.go`

- [ ] **Step 1: Create client_multiple.go**

```go
package lockman

import (
	"context"
	"fmt"

	"github.com/tuanuet/lockman/internal/sdk"
	"github.com/tuanuet/lockman/lockkit/definitions"
)

const maxMultipleKeys = 100

// RunMultiple acquires multiple keys of the same definition atomically (all-or-nothing)
// and executes the callback after all keys are acquired.
func (c *Client) RunMultiple(
	ctx context.Context,
	uc RunUseCase[T],
	fn func(ctx context.Context, lease Lease) error,
	input T,
	keys []string,
) error {
	if c == nil {
		return fmt.Errorf("lockman: client is nil")
	}
	if fn == nil {
		return fmt.Errorf("lockman: run multiple callback is required")
	}
	if c.shuttingDown.Load() {
		return ErrShuttingDown
	}

	if err := validateMultipleKeys(keys); err != nil {
		return err
	}

	identity, err := c.validateRunUseCase(ctx, uc)
	if err != nil {
		return err
	}
	if c.runtime == nil {
		return ErrUseCaseNotFound
	}

	definitionID := c.plan.definitionIDByUseCase[uc.core.name]
	if definitionID == "" {
		return ErrUseCaseNotFound
	}

	multipleReq := definitions.MultipleLockRequest{
		DefinitionID: definitionID,
		Keys:         keys,
		Ownership: definitions.OwnershipMeta{
			ServiceName: identity.Service,
			InstanceID:  identity.Instance,
			HandlerName: uc.core.name,
			OwnerID:     identity.OwnerID,
		},
	}

	err = c.runtime.ExecuteMultipleExclusive(ctx, multipleReq, func(ctx context.Context, lease definitions.LeaseContext) error {
		return fn(ctx, Lease{
			UseCase:      uc.core.name,
			ResourceKeys: append([]string(nil), lease.ResourceKeys...),
			LeaseTTL:     lease.LeaseTTL,
			Deadline:     lease.LeaseDeadline,
			FencingToken: lease.FencingToken,
		})
	})

	return mapEngineError(err, c.shuttingDown.Load())
}

// HoldMultiple acquires multiple keys of the same definition atomically and returns
// a single HoldHandle that manages all acquired keys.
func (c *Client) HoldMultiple(
	ctx context.Context,
	uc HoldUseCase[T],
	input T,
	keys []string,
) (HoldHandle, error) {
	if c == nil {
		return HoldHandle{}, fmt.Errorf("lockman: client is nil")
	}
	if c.shuttingDown.Load() {
		return HoldHandle{}, ErrShuttingDown
	}

	if err := validateMultipleKeys(keys); err != nil {
		return HoldHandle{}, err
	}

	identity, err := c.validateHoldUseCase(ctx, uc)
	if err != nil {
		return HoldHandle{}, err
	}
	if c.holds == nil {
		return HoldHandle{}, ErrUseCaseNotFound
	}

	definitionID := c.plan.definitionIDByUseCase[uc.core.name]
	if definitionID == "" {
		return HoldHandle{}, ErrUseCaseNotFound
	}

	token, err := sdk.EncodeHoldToken(keys, identity.OwnerID)
	if err != nil {
		return HoldHandle{}, fmt.Errorf("lockman: encode hold token: %w", ErrHoldTokenInvalid)
	}

	_, err = c.holds.Acquire(ctx, definitions.DetachedAcquireRequest{
		DefinitionID: definitionID,
		ResourceKeys: keys,
		OwnerID:      identity.OwnerID,
	})
	if err != nil {
		return HoldHandle{}, mapHoldAcquireError(err, c.shuttingDown.Load())
	}

	return HoldHandle{token: token}, nil
}

func validateMultipleKeys(keys []string) error {
	if len(keys) == 0 {
		return fmt.Errorf("lockman: keys must not be empty")
	}
	if len(keys) > maxMultipleKeys {
		return fmt.Errorf("lockman: keys must not exceed %d", maxMultipleKeys)
	}
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			return fmt.Errorf("lockman: duplicate key %q", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (c *Client) validateRunUseCase(ctx context.Context, uc RunUseCase[T]) (Identity, error) {
	if uc.core == nil {
		return Identity{}, ErrUseCaseNotFound
	}
	if !uc.core.boundToRegistry {
		return Identity{}, fmt.Errorf("lockman: use case %q is not registered: %w", uc.core.name, ErrUseCaseNotFound)
	}
	if c.registry == nil || sdk.RegistryLinkMismatch(c.registry.link, uc.core.registry.link) {
		return Identity{}, fmt.Errorf("lockman: use case %q belongs to a different registry: %w", uc.core.name, ErrRegistryMismatch)
	}

	return c.resolveIdentity(ctx, "")
}

func (c *Client) validateHoldUseCase(ctx context.Context, uc HoldUseCase[T]) (Identity, error) {
	if uc.core == nil {
		return Identity{}, ErrUseCaseNotFound
	}
	if !uc.core.boundToRegistry {
		return Identity{}, fmt.Errorf("lockman: use case %q is not registered: %w", uc.core.name, ErrUseCaseNotFound)
	}
	if c.registry == nil || sdk.RegistryLinkMismatch(c.registry.link, uc.core.registry.link) {
		return Identity{}, fmt.Errorf("lockman: use case %q belongs to a different registry: %w", uc.core.name, ErrRegistryMismatch)
	}

	return c.resolveIdentity(ctx, "")
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test . -run 'TestRunMultiple|TestHoldMultiple' -v`
Expected: All 9 tests PASS

- [ ] **Step 3: Run full root package tests**

Run: `go test . -v`
Expected: All tests PASS (existing + new)

- [ ] **Step 4: Commit**

```bash
git add client_multiple.go client_multiple_test.go
git commit -m "feat: add RunMultiple and HoldMultiple client methods"
```

---

## Chunk 3: Examples + Documentation

### Task 6: Create RunMultiple example

**Files:**
- Create: `examples/sdk/multiple-run/main.go`
- Reference: `examples/sdk/sync-transfer-funds/main.go`

- [ ] **Step 1: Create the example**

```go
//go:build lockman_examples

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/tuanuet/lockman"
	lockredis "github.com/tuanuet/lockman/backend/redis"
)

type batchOrderInput struct {
	OrderID string
}

var orderDef = lockman.DefineLock(
	"order",
	lockman.BindResourceID("order", func(in batchOrderInput) string { return in.OrderID }),
)

var batchProcess = lockman.DefineRunOn("batch_process", orderDef, lockman.TTL(5*time.Second))

func main() {
	client, err := redisClientFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "example failed: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	if err := run(os.Stdout, client); err != nil {
		fmt.Fprintf(os.Stderr, "example failed: %v\n", err)
		os.Exit(1)
	}
}

func run(out io.Writer, redisClient goredis.UniversalClient) error {
	reg := lockman.NewRegistry()
	if err := reg.Register(batchProcess); err != nil {
		return err
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "batch-worker"}),
		lockman.WithBackend(lockredis.New(redisClient, "")),
	)
	if err != nil {
		return err
	}
	defer client.Shutdown(context.Background())

	keys := []string{"order:1", "order:2", "order:3"}

	if err := client.RunMultiple(context.Background(), batchProcess, func(_ context.Context, lease lockman.Lease) error {
		joined := strings.Join(lease.ResourceKeys, ",")
		if _, err := fmt.Fprintf(out, "batch locked: %s\n", joined); err != nil {
			return err
		}
		_, err := fmt.Fprintf(out, "lease ttl: %s\n", lease.LeaseTTL)
		return err
	}, batchOrderInput{}, keys); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, "shutdown: ok"); err != nil {
		return err
	}
	return nil
}

func redisClientFromEnv() (*goredis.Client, error) {
	url := os.Getenv("LOCKMAN_REDIS_URL")
	if url == "" {
		url = "redis://127.0.0.1:6379/0"
	}
	opts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return goredis.NewClient(opts), nil
}
```

- [ ] **Step 2: Compile check**

Run: `go test -tags lockman_examples ./examples/sdk/multiple-run/... -run '^$'`
Expected: PASS (compiles)

- [ ] **Step 3: Commit**

```bash
git add examples/sdk/multiple-run/main.go
git commit -m "docs: add RunMultiple example"
```

---

### Task 7: Create HoldMultiple example

**Files:**
- Create: `examples/sdk/multiple-hold/main.go`
- Reference: `examples/sdk/manual-hold/main.go`

- [ ] **Step 1: Create the example**

```go
//go:build lockman_examples

package main

import (
	"context"
	"fmt"
	"io"
	"os"

	goredis "github.com/redis/go-redis/v9"

	"github.com/tuanuet/lockman"
	lockredis "github.com/tuanuet/lockman/backend/redis"
)

type reserveInput struct {
	SlotID string
}

var slotDef = lockman.DefineLock(
	"slot",
	lockman.BindResourceID("slot", func(in reserveInput) string { return in.SlotID }),
)

var reserveSlots = lockman.DefineHoldOn("reserve_slots", slotDef, lockman.TTL(30*time.Second))

func main() {
	client, err := redisClientFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "example failed: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	if err := run(os.Stdout, client); err != nil {
		fmt.Fprintf(os.Stderr, "example failed: %v\n", err)
		os.Exit(1)
	}
}

func run(out io.Writer, redisClient goredis.UniversalClient) error {
	reg := lockman.NewRegistry()
	if err := reg.Register(reserveSlots); err != nil {
		return err
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "warehouse-api"}),
		lockman.WithBackend(lockredis.New(redisClient, "")),
	)
	if err != nil {
		return err
	}
	defer client.Shutdown(context.Background())

	keys := []string{"slot:A", "slot:B", "slot:C"}

	handle, err := client.HoldMultiple(context.Background(), reserveSlots, reserveInput{}, keys)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(out, "hold keys: %s\n", keys); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "hold token: %s\n", handle.Token()); err != nil {
		return err
	}

	if err := client.Forfeit(context.Background(), reserveSlots.ForfeitWith(handle.Token())); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(out, "forfeit: ok"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "shutdown: ok"); err != nil {
		return err
	}

	return nil
}

func redisClientFromEnv() (*goredis.Client, error) {
	url := os.Getenv("LOCKMAN_REDIS_URL")
	if url == "" {
		url = "redis://127.0.0.1:6379/0"
	}
	opts, err := goredis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	return goredis.NewClient(opts), nil
}
```

- [ ] **Step 2: Compile check**

Run: `go test -tags lockman_examples ./examples/sdk/multiple-hold/... -run '^$'`
Expected: PASS (compiles)

- [ ] **Step 3: Commit**

```bash
git add examples/sdk/multiple-hold/main.go
git commit -m "docs: add HoldMultiple example"
```

---

### Task 8: Create documentation

**Files:**
- Create: `docs/multiple-lock.md`

- [ ] **Step 1: Create the documentation**

```markdown
# Multiple Lock

Multiple lock acquires **multiple keys of the same definition** in one atomic all-or-nothing operation.

## When to Use

Use multiple lock when you need exclusive access to several resources of the **same type** at once:

- Batch processing: lock 5 orders before running a batch update
- Multi-resource reservation: hold several warehouse slots for a manual workflow
- Dynamic key sets: the number of keys is not known at definition time

## Multiple vs Composite

| | Composite | Multiple |
|---|---|---|
| Definitions | N different definitions | 1 definition |
| Keys | 1 key per definition | N keys, same definition |
| Key count | Fixed at definition time | Dynamic at call time |
| Use case | Cross-resource coordination | Batch same-type operations |
| Error semantics | Per-member errors | All-or-nothing |

## RunMultiple

Acquires multiple keys, runs a callback, then releases all keys.

```go
orderDef := lockman.DefineLock(
    "order",
    lockman.BindResourceID("order", func(in BatchInput) string { return in.OrderID }),
)
batchUC := lockman.DefineRunOn("batch_process", orderDef)

err := client.RunMultiple(ctx, batchUC, func(ctx context.Context, lease lockman.Lease) error {
    // lease.ResourceKeys = ["order:1", "order:2", "order:3"]
    return processBatch(ctx, lease.ResourceKeys)
}, input, []string{"order:1", "order:2", "order:3"})
```

### Behavior

- **All-or-nothing**: if any key fails to acquire, all previously acquired keys are released
- **Canonical ordering**: keys are sorted alphabetically before acquisition (prevents deadlocks)
- **Overlap rejection**: if any key overlaps with existing locks, the entire operation is rejected
- **Max keys**: 100 keys per call

## HoldMultiple

Acquires multiple keys and returns a single `HoldHandle`. Keys remain locked until `Forfeit` is called.

```go
slotDef := lockman.DefineLock(
    "slot",
    lockman.BindResourceID("slot", func(in ReserveInput) string { return in.SlotID }),
)
holdUC := lockman.DefineHoldOn("reserve_slots", slotDef)

handle, err := client.HoldMultiple(ctx, holdUC, input, []string{"slot:A", "slot:B", "slot:C"})
// ... manual workflow steps ...
client.Forfeit(ctx, holdUC.ForfeitWith(handle.Token()))
```

### Behavior

- Same all-or-nothing acquisition as `RunMultiple`
- Single `HoldHandle` manages all keys
- Renewal is handled by the hold manager for all keys
- `Forfeit` releases all keys at once

## Validation

| Condition | Error |
|---|---|
| Empty keys list | `keys must not be empty` |
| Duplicate keys | `duplicate key "..."` |
| More than 100 keys | `keys must not exceed 100` |
| Strict definition | `lockman: backend lacks required capability` |
| Use case not registered | `lockman: use case not found` |
| Registry mismatch | `lockman: use case does not belong to this registry` |

## Examples

- `examples/sdk/multiple-run/` — RunMultiple with Redis backend
- `examples/sdk/multiple-hold/` — HoldMultiple with Redis backend
```

- [ ] **Step 2: Commit**

```bash
git add docs/multiple-lock.md
git commit -m "docs: add multiple lock documentation"
```

---

## Chunk 4: Verification + CI Parity

### Task 9: Full verification

- [ ] **Step 1: Run all workspace tests**

Run: `go test ./...`
Expected: All PASS

- [ ] **Step 2: Run tests without workspace mode**

Run: `GOWORK=off go test ./...`
Expected: All PASS

- [ ] **Step 3: Run module-specific tests**

Run: `go test ./backend/redis/...`
Run: `go test ./idempotency/redis/...`
Run: `go test ./guard/postgres/...`
Expected: All PASS

- [ ] **Step 4: Compile examples**

Run: `go test -tags lockman_examples ./examples/... -run '^$'`
Expected: PASS

- [ ] **Step 5: Run lint**

Run: `make lint`
Run: `gofmt -l .`
Expected: No output from gofmt

- [ ] **Step 6: Final commit if any lint fixes needed**

```bash
git add -A && git commit -m "style: apply gofmt"
```

---

## Execution Handoff

Plan complete. Ready to execute?