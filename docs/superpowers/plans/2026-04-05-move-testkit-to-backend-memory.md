# Move Testkit to Public backend/memory Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move `lockkit/testkit` to `backend/memory` as a public, importable in-memory backend for unit testing without Redis.

**Architecture:** The `backend/memory` package lives under the root module (no separate `go.mod` needed — no external dependencies). It implements `backend.Driver`, `backend.StrictDriver`, and `backend.LineageDriver`. All existing imports across ~30 files are updated. The old `lockkit/testkit/` directory is deleted.

**Tech Stack:** Go 1.22, root module `github.com/tuanuet/lockman`, `backend` contracts package

---

## File Structure

| Action | File | Responsibility |
|--------|------|----------------|
| Create | `backend/memory/driver.go` | `MemoryDriver` implementation (moved from `lockkit/testkit/memory_driver.go`, swap `lockerrors.ErrOverlapRejected` → `backend.ErrOverlapRejected`) |
| Create | `backend/memory/driver_test.go` | Tests (moved from `lockkit/testkit/memory_driver_test.go`, same import swap) |
| Create | `backend/memory/assertions.go` | Test helpers (moved from `lockkit/testkit/assertions.go`) |
| Modify | All ~30 files importing `lockkit/testkit` | Update import to `backend/memory` |
| Modify | `SKILL.md` | Update doc reference |
| Delete | `lockkit/testkit/` | Entire directory |

---

## Chunk 1: Create backend/memory package

### Task 1: Create backend/memory/driver.go

**Files:**
- Create: `backend/memory/driver.go`

- [ ] **Step 1: Create the driver file**

Copy `lockkit/testkit/memory_driver.go` to `backend/memory/driver.go` with these changes:
1. Change package name from `testkit` to `memory`
2. Remove import `"github.com/tuanuet/lockman/lockkit/errors"`
3. Replace `lockerrors.ErrOverlapRejected` with `backend.ErrOverlapRejected` (2 occurrences in `rejectLineageConflict`)

```go
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/tuanuet/lockman/backend"
)

// MemoryDriver is a naive single-resource driver useful for tests and local builds.
type MemoryDriver struct {
	mu     sync.Mutex
	leases map[string]backend.LeaseRecord

	// strictCounters tracks fencing tokens by strict boundary (definition + resource key).
	strictCounters map[string]uint64
	// strictLeases tracks active strict lease metadata by strict boundary.
	strictLeases map[string]strictLeaseState

	// lineageLeases stores lineage metadata by lease id so descendant membership can be pruned reliably.
	lineageLeases map[string]lineageLeaseState

	// descendantsByAncestor maps a stable ancestor key (definition + resource key) to active descendant lease ids.
	descendantsByAncestor map[string]map[string]time.Time
}

type lineageLeaseState struct {
	lease    backend.LeaseRecord
	lineage  backend.LineageLeaseMeta
	expireAt time.Time
}

type strictLeaseState struct {
	lease        backend.LeaseRecord
	fencingToken uint64
}

// NewMemoryDriver returns a ready-to-use in-memory driver.
func NewMemoryDriver() *MemoryDriver {
	return &MemoryDriver{
		leases:                make(map[string]backend.LeaseRecord),
		strictCounters:        make(map[string]uint64),
		strictLeases:          make(map[string]strictLeaseState),
		lineageLeases:         make(map[string]lineageLeaseState),
		descendantsByAncestor: make(map[string]map[string]time.Time),
	}
}

// Acquire attempts to claim a single resource. It rejects already held leases.
func (m *MemoryDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	if len(req.ResourceKeys) != 1 {
		return backend.LeaseRecord{}, backend.ErrInvalidRequest
	}
	if req.LeaseTTL <= 0 {
		return backend.LeaseRecord{}, backend.ErrInvalidRequest
	}

	key := req.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	if existing, ok := m.leases[key]; ok {
		if !existing.IsExpired(now) {
			return backend.LeaseRecord{}, backend.ErrLeaseAlreadyHeld
		}
		delete(m.leases, key)
	}

	lease := backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: append([]string(nil), req.ResourceKeys...),
		OwnerID:      req.OwnerID,
		LeaseTTL:     req.LeaseTTL,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(req.LeaseTTL),
	}
	m.leases[key] = lease
	return lease, nil
}

// Renew refreshes an existing lease while the owner still holds it.
func (m *MemoryDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	if len(lease.ResourceKeys) != 1 {
		return backend.LeaseRecord{}, backend.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	existing, ok := m.leases[key]
	if !ok {
		return backend.LeaseRecord{}, backend.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return backend.LeaseRecord{}, backend.ErrLeaseOwnerMismatch
	}
	if existing.IsExpired(now) {
		delete(m.leases, key)
		return backend.LeaseRecord{}, backend.ErrLeaseExpired
	}

	ttl := lease.LeaseTTL
	if ttl <= 0 {
		ttl = existing.LeaseTTL
	}
	existing.LeaseTTL = ttl
	existing.AcquiredAt = now
	existing.ExpiresAt = now.Add(ttl)
	m.leases[key] = existing
	return existing, nil
}

// Release removes the lease so another owner can claim the resource.
func (m *MemoryDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	if len(lease.ResourceKeys) != 1 {
		return backend.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(time.Now())

	existing, ok := m.leases[key]
	if !ok {
		return backend.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return backend.ErrLeaseOwnerMismatch
	}

	delete(m.leases, key)
	return nil
}

// AcquireStrict attempts to claim a single resource and returns a fencing token.
func (m *MemoryDriver) AcquireStrict(ctx context.Context, req backend.StrictAcquireRequest) (backend.FencedLeaseRecord, error) {
	if req.ResourceKey == "" {
		return backend.FencedLeaseRecord{}, backend.ErrInvalidRequest
	}
	if req.LeaseTTL <= 0 {
		return backend.FencedLeaseRecord{}, backend.ErrInvalidRequest
	}

	key := req.ResourceKey
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	if existing, ok := m.leases[key]; ok {
		if !existing.IsExpired(now) {
			return backend.FencedLeaseRecord{}, backend.ErrLeaseAlreadyHeld
		}
		delete(m.leases, key)
		delete(m.strictLeases, strictBoundaryKey(existing.DefinitionID, key))
	}

	lease := backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{req.ResourceKey},
		OwnerID:      req.OwnerID,
		LeaseTTL:     req.LeaseTTL,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(req.LeaseTTL),
	}

	boundary := strictBoundaryKey(req.DefinitionID, req.ResourceKey)
	nextToken := m.strictCounters[boundary] + 1
	m.strictCounters[boundary] = nextToken

	m.leases[key] = lease
	m.strictLeases[boundary] = strictLeaseState{
		lease:        lease,
		fencingToken: nextToken,
	}

	return backend.FencedLeaseRecord{
		Lease:        lease,
		FencingToken: nextToken,
	}, nil
}

// RenewStrict refreshes an existing strict lease while preserving its fencing token.
func (m *MemoryDriver) RenewStrict(ctx context.Context, lease backend.LeaseRecord, fencingToken uint64) (backend.FencedLeaseRecord, error) {
	if len(lease.ResourceKeys) != 1 {
		return backend.FencedLeaseRecord{}, backend.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]
	boundary := strictBoundaryKey(lease.DefinitionID, key)
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	existing, ok := m.leases[key]
	if !ok {
		return backend.FencedLeaseRecord{}, backend.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return backend.FencedLeaseRecord{}, backend.ErrLeaseOwnerMismatch
	}
	if existing.IsExpired(now) {
		delete(m.leases, key)
		delete(m.strictLeases, boundary)
		return backend.FencedLeaseRecord{}, backend.ErrLeaseExpired
	}
	if existing.DefinitionID != lease.DefinitionID {
		return backend.FencedLeaseRecord{}, backend.ErrLeaseOwnerMismatch
	}

	strictState, ok := m.strictLeases[boundary]
	if !ok {
		return backend.FencedLeaseRecord{}, backend.ErrLeaseNotFound
	}
	if strictState.lease.OwnerID != lease.OwnerID || strictState.fencingToken != fencingToken {
		return backend.FencedLeaseRecord{}, backend.ErrLeaseOwnerMismatch
	}

	ttl := lease.LeaseTTL
	if ttl <= 0 {
		ttl = existing.LeaseTTL
	}
	existing.LeaseTTL = ttl
	existing.AcquiredAt = now
	existing.ExpiresAt = now.Add(ttl)
	m.leases[key] = existing

	strictState.lease = existing
	m.strictLeases[boundary] = strictState

	return backend.FencedLeaseRecord{
		Lease:        existing,
		FencingToken: strictState.fencingToken,
	}, nil
}

// ReleaseStrict removes a strict lease after owner and fencing token validation.
func (m *MemoryDriver) ReleaseStrict(ctx context.Context, lease backend.LeaseRecord, fencingToken uint64) error {
	if len(lease.ResourceKeys) != 1 {
		return backend.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]
	boundary := strictBoundaryKey(lease.DefinitionID, key)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(time.Now())

	existing, ok := m.leases[key]
	if !ok {
		return backend.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return backend.ErrLeaseOwnerMismatch
	}
	if existing.DefinitionID != lease.DefinitionID {
		return backend.ErrLeaseOwnerMismatch
	}

	strictState, ok := m.strictLeases[boundary]
	if !ok {
		return backend.ErrLeaseNotFound
	}
	if strictState.lease.OwnerID != lease.OwnerID || strictState.fencingToken != fencingToken {
		return backend.ErrLeaseOwnerMismatch
	}

	delete(m.leases, key)
	delete(m.strictLeases, boundary)
	return nil
}

// CheckPresence reports whether the resource is currently held.
func (m *MemoryDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	if len(req.ResourceKeys) != 1 {
		return backend.PresenceRecord{}, backend.ErrInvalidRequest
	}

	key := req.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	record := backend.PresenceRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{key},
	}

	if lease, ok := m.leases[key]; ok {
		if lease.IsExpired(now) {
			delete(m.leases, key)
			return record, nil
		}
		record.Lease = lease
		record.Present = true
		record.DefinitionID = lease.DefinitionID
		record.ResourceKeys = append([]string(nil), lease.ResourceKeys...)
		return record, nil
	}

	return record, nil
}

func (m *MemoryDriver) AcquireWithLineage(ctx context.Context, req backend.LineageAcquireRequest) (backend.LeaseRecord, error) {
	if req.ResourceKey == "" {
		return backend.LeaseRecord{}, backend.ErrInvalidRequest
	}
	if req.LeaseTTL <= 0 {
		return backend.LeaseRecord{}, backend.ErrInvalidRequest
	}
	if req.Lineage.LeaseID == "" {
		return backend.LeaseRecord{}, backend.ErrInvalidRequest
	}

	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)
	if err := m.rejectLineageConflict(now, req); err != nil {
		return backend.LeaseRecord{}, err
	}

	lease := backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{req.ResourceKey},
		OwnerID:      req.OwnerID,
		LeaseTTL:     req.LeaseTTL,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(req.LeaseTTL),
	}

	meta := backend.LineageLeaseMeta{
		LeaseID:      req.Lineage.LeaseID,
		Kind:         req.Lineage.Kind,
		AncestorKeys: cloneAncestorKeys(req.Lineage.AncestorKeys),
	}

	m.storeLeaseAndMembership(lease, meta)
	return lease, nil
}

func (m *MemoryDriver) RenewWithLineage(ctx context.Context, lease backend.LeaseRecord, lineage backend.LineageLeaseMeta) (backend.LeaseRecord, backend.LineageLeaseMeta, error) {
	if len(lease.ResourceKeys) != 1 {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrInvalidRequest
	}
	if lineage.LeaseID == "" {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	existing, ok := m.leases[key]
	if !ok {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseOwnerMismatch
	}
	if existing.IsExpired(now) {
		delete(m.leases, key)
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseExpired
	}

	state, ok := m.lineageLeases[lineage.LeaseID]
	if !ok {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseExpired
	}
	for _, ancestor := range lineage.AncestorKeys {
		ancestorKey := formatAncestorKey(ancestor)
		members := m.descendantsByAncestor[ancestorKey]
		if members == nil {
			return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseExpired
		}
		if _, ok := members[lineage.LeaseID]; !ok {
			return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, backend.ErrLeaseExpired
		}
	}

	ttl := lease.LeaseTTL
	if ttl <= 0 {
		ttl = existing.LeaseTTL
	}
	existing.LeaseTTL = ttl
	existing.AcquiredAt = now
	existing.ExpiresAt = now.Add(ttl)
	m.leases[key] = existing

	state.lease = existing
	state.expireAt = existing.ExpiresAt
	m.lineageLeases[lineage.LeaseID] = state

	for _, ancestor := range lineage.AncestorKeys {
		ancestorKey := formatAncestorKey(ancestor)
		members := m.descendantsByAncestor[ancestorKey]
		if members == nil {
			continue
		}
		if _, ok := members[lineage.LeaseID]; ok {
			members[lineage.LeaseID] = existing.ExpiresAt
		}
	}

	return existing, backend.LineageLeaseMeta{
		LeaseID:      lineage.LeaseID,
		Kind:         lineage.Kind,
		AncestorKeys: cloneAncestorKeys(lineage.AncestorKeys),
	}, nil
}

func (m *MemoryDriver) ReleaseWithLineage(ctx context.Context, lease backend.LeaseRecord, lineage backend.LineageLeaseMeta) error {
	if len(lease.ResourceKeys) != 1 {
		return backend.ErrInvalidRequest
	}
	if lineage.LeaseID == "" {
		return backend.ErrInvalidRequest
	}

	key := lease.ResourceKeys[0]
	now := time.Now()

	m.mu.Lock()
	defer m.mu.Unlock()

	m.pruneExpired(now)

	existing, ok := m.leases[key]
	if !ok {
		return backend.ErrLeaseNotFound
	}
	if existing.OwnerID != lease.OwnerID {
		return backend.ErrLeaseOwnerMismatch
	}

	delete(m.leases, key)
	delete(m.lineageLeases, lineage.LeaseID)
	for _, ancestor := range lineage.AncestorKeys {
		ancestorKey := formatAncestorKey(ancestor)
		members := m.descendantsByAncestor[ancestorKey]
		if members == nil {
			continue
		}
		delete(members, lineage.LeaseID)
		if len(members) == 0 {
			delete(m.descendantsByAncestor, ancestorKey)
		}
	}

	return nil
}

// Ping always succeeds for the in-memory driver.
func (m *MemoryDriver) Ping(ctx context.Context) error {
	return nil
}

func (m *MemoryDriver) rejectLineageConflict(now time.Time, req backend.LineageAcquireRequest) error {
	if existing, ok := m.leases[req.ResourceKey]; ok && !existing.IsExpired(now) {
		return backend.ErrLeaseAlreadyHeld
	}

	switch req.Lineage.Kind {
	case backend.KindParent:
		ancestorKey := formatAncestorKey(backend.AncestorKey{
			DefinitionID: req.DefinitionID,
			ResourceKey:  req.ResourceKey,
		})
		for _, expireAt := range m.descendantsByAncestor[ancestorKey] {
			if now.Before(expireAt) {
				return backend.ErrOverlapRejected
			}
		}
	case backend.KindChild:
		for _, ancestor := range req.Lineage.AncestorKeys {
			if existing, ok := m.leases[ancestor.ResourceKey]; ok && !existing.IsExpired(now) {
				return backend.ErrOverlapRejected
			}
		}
	}

	return nil
}

func (m *MemoryDriver) storeLeaseAndMembership(lease backend.LeaseRecord, lineage backend.LineageLeaseMeta) {
	key := lease.ResourceKeys[0]
	m.leases[key] = lease
	m.lineageLeases[lineage.LeaseID] = lineageLeaseState{
		lease:    lease,
		lineage:  lineage,
		expireAt: lease.ExpiresAt,
	}

	for _, ancestor := range lineage.AncestorKeys {
		ancestorKey := formatAncestorKey(ancestor)
		members := m.descendantsByAncestor[ancestorKey]
		if members == nil {
			members = make(map[string]time.Time)
			m.descendantsByAncestor[ancestorKey] = members
		}
		members[lineage.LeaseID] = lease.ExpiresAt
	}
}

func (m *MemoryDriver) pruneExpired(now time.Time) {
	for key, lease := range m.leases {
		if lease.IsExpired(now) {
			delete(m.leases, key)
			delete(m.strictLeases, strictBoundaryKey(lease.DefinitionID, key))
		}
	}

	for leaseID, state := range m.lineageLeases {
		if now.After(state.expireAt) || state.lease.IsExpired(now) {
			delete(m.lineageLeases, leaseID)
			for _, ancestor := range state.lineage.AncestorKeys {
				ancestorKey := formatAncestorKey(ancestor)
				members := m.descendantsByAncestor[ancestorKey]
				if members == nil {
					continue
				}
				delete(members, leaseID)
				if len(members) == 0 {
					delete(m.descendantsByAncestor, ancestorKey)
				}
			}
		}
	}
}

func strictBoundaryKey(definitionID, resourceKey string) string {
	return definitionID + "\x00" + resourceKey
}

func formatAncestorKey(key backend.AncestorKey) string {
	return key.DefinitionID + "\x00" + key.ResourceKey
}

func cloneAncestorKeys(input []backend.AncestorKey) []backend.AncestorKey {
	if len(input) == 0 {
		return nil
	}
	out := make([]backend.AncestorKey, len(input))
	copy(out, input)
	return out
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./backend/memory`
Expected: no errors

---

### Task 2: Create backend/memory/driver_test.go

**Files:**
- Create: `backend/memory/driver_test.go`

- [ ] **Step 1: Create the test file**

Copy `lockkit/testkit/memory_driver_test.go` to `backend/memory/driver_test.go` with these changes:
1. Change package name from `testkit` to `memory`
2. Remove import `lockerrors "github.com/tuanuet/lockman/lockkit/errors"`
3. Replace `lockerrors.ErrOverlapRejected` with `backend.ErrOverlapRejected` (2 occurrences)

```go
package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend"
)

func requireStrictDriver(t *testing.T, driver *MemoryDriver) backend.StrictDriver {
	t.Helper()

	strict, ok := any(driver).(backend.StrictDriver)
	if !ok {
		t.Fatal("memory driver must implement backend.StrictDriver")
	}
	return strict
}

func TestMemoryDriverAcquireStrictIssuesIncreasingTokens(t *testing.T) {
	driver := NewMemoryDriver()
	strict := requireStrictDriver(t, driver)
	ctx := context.Background()

	first, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict first returned error: %v", err)
	}
	if err := strict.ReleaseStrict(ctx, first.Lease, first.FencingToken); err != nil {
		t.Fatalf("ReleaseStrict first returned error: %v", err)
	}

	second, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
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

func TestMemoryDriverRenewStrictPreservesToken(t *testing.T) {
	driver := NewMemoryDriver()
	strict := requireStrictDriver(t, driver)
	ctx := context.Background()

	acquired, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("AcquireStrict returned error: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	acquired.Lease.LeaseTTL = 40 * time.Millisecond
	renewed, err := strict.RenewStrict(ctx, acquired.Lease, acquired.FencingToken)
	if err != nil {
		t.Fatalf("RenewStrict returned error: %v", err)
	}
	if renewed.FencingToken != acquired.FencingToken {
		t.Fatalf("expected RenewStrict to preserve token %d, got %d", acquired.FencingToken, renewed.FencingToken)
	}
	if !renewed.Lease.ExpiresAt.After(acquired.Lease.ExpiresAt) {
		t.Fatalf("expected renewed expiry after %v, got %v", acquired.Lease.ExpiresAt, renewed.Lease.ExpiresAt)
	}
}

func TestMemoryDriverRenewStrictRejectsWrongToken(t *testing.T) {
	driver := NewMemoryDriver()
	strict := requireStrictDriver(t, driver)
	ctx := context.Background()

	acquired, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict returned error: %v", err)
	}

	_, err = strict.RenewStrict(ctx, acquired.Lease, acquired.FencingToken+1)
	if !errors.Is(err, backend.ErrLeaseOwnerMismatch) {
		t.Fatalf("expected ErrLeaseOwnerMismatch for wrong renew token, got %v", err)
	}
}

func TestMemoryDriverReleaseStrictRejectsWrongToken(t *testing.T) {
	driver := NewMemoryDriver()
	strict := requireStrictDriver(t, driver)
	ctx := context.Background()

	acquired, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict returned error: %v", err)
	}

	err = strict.ReleaseStrict(ctx, acquired.Lease, acquired.FencingToken+1)
	if !errors.Is(err, backend.ErrLeaseOwnerMismatch) {
		t.Fatalf("expected ErrLeaseOwnerMismatch for wrong token, got %v", err)
	}
}

func TestMemoryDriverAcquireStrictCounterIsScopedByDefinitionAndResource(t *testing.T) {
	driver := NewMemoryDriver()
	strict := requireStrictDriver(t, driver)
	ctx := context.Background()

	defA1, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict.a",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict defA returned error: %v", err)
	}
	if err := strict.ReleaseStrict(ctx, defA1.Lease, defA1.FencingToken); err != nil {
		t.Fatalf("ReleaseStrict defA returned error: %v", err)
	}

	defB1, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict.b",
		ResourceKey:  "order:123",
		OwnerID:      "worker-b",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict defB returned error: %v", err)
	}
	if defB1.FencingToken != 1 {
		t.Fatalf("expected independent counter for definition/resource boundary, got %d", defB1.FencingToken)
	}
}

func TestMemoryDriverReleaseStrictRejectsStaleLeaseFromDifferentDefinitionBoundary(t *testing.T) {
	driver := NewMemoryDriver()
	strict := requireStrictDriver(t, driver)
	ctx := context.Background()

	first, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict.a",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict first returned error: %v", err)
	}
	if err := strict.ReleaseStrict(ctx, first.Lease, first.FencingToken); err != nil {
		t.Fatalf("ReleaseStrict first returned error: %v", err)
	}

	second, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict.b",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict second returned error: %v", err)
	}

	err = strict.ReleaseStrict(ctx, first.Lease, first.FencingToken)
	if !errors.Is(err, backend.ErrLeaseOwnerMismatch) {
		t.Fatalf("expected stale cross-definition release to fail, got %v", err)
	}

	if err := strict.ReleaseStrict(ctx, second.Lease, second.FencingToken); err != nil {
		t.Fatalf("ReleaseStrict second returned error: %v", err)
	}
}

func TestMemoryDriverAcquireStrictRejectsEmptyResourceKey(t *testing.T) {
	driver := NewMemoryDriver()
	strict := requireStrictDriver(t, driver)

	_, err := strict.AcquireStrict(context.Background(), backend.StrictAcquireRequest{
		DefinitionID: "order.strict",
		ResourceKey:  "",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if !errors.Is(err, backend.ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest for empty resource key, got %v", err)
	}
}

func TestMemoryDriverAcquireAndRelease(t *testing.T) {
	driver := NewMemoryDriver()

	lease, err := driver.Acquire(context.Background(), backend.AcquireRequest{
		DefinitionID: "OrderLock",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "svc-a:instance-1",
		LeaseTTL:     30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	AssertSingleResourceLease(t, lease, "OrderLock", "svc-a:instance-1", "order:123")

	if err := driver.Release(context.Background(), lease); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}

	presence, err := driver.CheckPresence(context.Background(), backend.PresenceRequest{
		DefinitionID: "OrderLock",
		ResourceKeys: []string{"order:123"},
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}

	if presence.Present {
		t.Fatalf("expected resource to be absent after release, got %+v", presence)
	}
}

func TestMemoryDriverAcquireWithLineageRejectsParentWhileChildHeld(t *testing.T) {
	driver := NewMemoryDriver()

	childReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}
	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}
	defer func() {
		_ = driver.ReleaseWithLineage(context.Background(), childLease, childReq.Lineage)
	}()

	_, err = driver.AcquireWithLineage(context.Background(), backend.LineageAcquireRequest{
		DefinitionID: "order",
		ResourceKey:  "order:123",
		OwnerID:      "worker-b",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    backend.KindParent,
		},
	})
	if !errors.Is(err, backend.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection, got %v", err)
	}
}

func TestCheckPresenceRemainsExactKeyOnlyWithActiveChild(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}
	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}
	defer func() {
		_ = driver.ReleaseWithLineage(context.Background(), childLease, childReq.Lineage)
	}()

	record, err := driver.CheckPresence(context.Background(), backend.PresenceRequest{
		DefinitionID: "order",
		ResourceKeys: []string{"order:123"},
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}
	if record.Present {
		t.Fatalf("expected exact-key presence only, got %#v", record)
	}
}

func TestMemoryDriverRenewWithLineageExtendsDescendantMembershipTTL(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Millisecond,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}

	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}
	originalExpireAt := driver.lineageLeases[childReq.Lineage.LeaseID].expireAt

	time.Sleep(10 * time.Millisecond)

	childLease.LeaseTTL = 90 * time.Millisecond
	renewedLease, renewedMeta, err := driver.RenewWithLineage(context.Background(), childLease, childReq.Lineage)
	if err != nil {
		t.Fatalf("renew failed: %v", err)
	}
	defer func() {
		_ = driver.ReleaseWithLineage(context.Background(), renewedLease, renewedMeta)
	}()

	if !driver.lineageLeases[childReq.Lineage.LeaseID].expireAt.After(originalExpireAt) {
		t.Fatalf("expected lineage lease expiry to extend beyond %v, got %v", originalExpireAt, driver.lineageLeases[childReq.Lineage.LeaseID].expireAt)
	}

	ancestorKey := formatAncestorKey(childReq.Lineage.AncestorKeys[0])
	if got := driver.descendantsByAncestor[ancestorKey][childReq.Lineage.LeaseID]; !got.Equal(renewedLease.ExpiresAt) {
		t.Fatalf("expected descendant membership expiry %v, got %v", renewedLease.ExpiresAt, got)
	}
}

func TestMemoryDriverRenewWithLineageFailsWhenLineageStateMissing(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     25 * time.Millisecond,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}

	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}

	delete(driver.lineageLeases, childReq.Lineage.LeaseID)

	childLease.LeaseTTL = 80 * time.Millisecond
	_, _, err = driver.RenewWithLineage(context.Background(), childLease, childReq.Lineage)
	if !errors.Is(err, backend.ErrLeaseExpired) {
		t.Fatalf("expected renew failure when lineage state is missing, got %v", err)
	}

	time.Sleep(35 * time.Millisecond)

	reacquired, err := driver.AcquireWithLineage(context.Background(), backend.LineageAcquireRequest{
		DefinitionID: childReq.DefinitionID,
		ResourceKey:  childReq.ResourceKey,
		OwnerID:      "worker-b",
		LeaseTTL:     50 * time.Millisecond,
		Lineage: backend.LineageLeaseMeta{
			LeaseID:      "lease-child-reacquired",
			Kind:         childReq.Lineage.Kind,
			AncestorKeys: append([]backend.AncestorKey(nil), childReq.Lineage.AncestorKeys...),
		},
	})
	if err != nil {
		t.Fatalf("expected child lease to expire on original ttl, got %v", err)
	}
	if err := driver.ReleaseWithLineage(context.Background(), reacquired, backend.LineageLeaseMeta{
		LeaseID:      "lease-child-reacquired",
		Kind:         childReq.Lineage.Kind,
		AncestorKeys: append([]backend.AncestorKey(nil), childReq.Lineage.AncestorKeys...),
	}); err != nil {
		t.Fatalf("release reacquired child failed: %v", err)
	}
}

func TestMemoryDriverRenewWithLineageFailsWhenAncestorMembershipMissing(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     25 * time.Millisecond,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}

	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}

	ancestorKey := formatAncestorKey(childReq.Lineage.AncestorKeys[0])
	delete(driver.descendantsByAncestor[ancestorKey], childReq.Lineage.LeaseID)

	childLease.LeaseTTL = 80 * time.Millisecond
	_, _, err = driver.RenewWithLineage(context.Background(), childLease, childReq.Lineage)
	if !errors.Is(err, backend.ErrLeaseExpired) {
		t.Fatalf("expected renew failure when ancestor membership is missing, got %v", err)
	}

	time.Sleep(35 * time.Millisecond)

	reacquired, err := driver.AcquireWithLineage(context.Background(), backend.LineageAcquireRequest{
		DefinitionID: childReq.DefinitionID,
		ResourceKey:  childReq.ResourceKey,
		OwnerID:      "worker-b",
		LeaseTTL:     50 * time.Millisecond,
		Lineage: backend.LineageLeaseMeta{
			LeaseID:      "lease-child-reacquired",
			Kind:         childReq.Lineage.Kind,
			AncestorKeys: append([]backend.AncestorKey(nil), childReq.Lineage.AncestorKeys...),
		},
	})
	if err != nil {
		t.Fatalf("expected child lease to expire on original ttl, got %v", err)
	}
	if err := driver.ReleaseWithLineage(context.Background(), reacquired, backend.LineageLeaseMeta{
		LeaseID:      "lease-child-reacquired",
		Kind:         childReq.Lineage.Kind,
		AncestorKeys: append([]backend.AncestorKey(nil), childReq.Lineage.AncestorKeys...),
	}); err != nil {
		t.Fatalf("release reacquired child failed: %v", err)
	}
}

func TestMemoryDriverReleaseWithLineageClearsDescendantMembership(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}

	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}
	if err := driver.ReleaseWithLineage(context.Background(), childLease, childReq.Lineage); err != nil {
		t.Fatalf("release failed: %v", err)
	}

	ancestorKey := formatAncestorKey(childReq.Lineage.AncestorKeys[0])
	if got := len(driver.descendantsByAncestor[ancestorKey]); got != 0 {
		t.Fatalf("expected descendant membership cleanup, got %d entries", got)
	}

	parentLease, err := driver.AcquireWithLineage(context.Background(), backend.LineageAcquireRequest{
		DefinitionID: "order",
		ResourceKey:  "order:123",
		OwnerID:      "worker-b",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    backend.KindParent,
		},
	})
	if err != nil {
		t.Fatalf("expected parent acquire after child release, got %v", err)
	}
	if err := driver.ReleaseWithLineage(context.Background(), parentLease, backend.LineageLeaseMeta{
		LeaseID: "lease-parent",
		Kind:    backend.KindParent,
	}); err != nil {
		t.Fatalf("parent release failed: %v", err)
	}
}

func TestMemoryDriverAcquireWithLineagePrunesExpiredDescendantMembership(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     20 * time.Millisecond,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}

	if _, err := driver.AcquireWithLineage(context.Background(), childReq); err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}

	time.Sleep(30 * time.Millisecond)

	parentLease, err := driver.AcquireWithLineage(context.Background(), backend.LineageAcquireRequest{
		DefinitionID: "order",
		ResourceKey:  "order:123",
		OwnerID:      "worker-b",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    backend.KindParent,
		},
	})
	if err != nil {
		t.Fatalf("expected parent acquire after child expiry, got %v", err)
	}
	defer func() {
		_ = driver.ReleaseWithLineage(context.Background(), parentLease, backend.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    backend.KindParent,
		})
	}()

	ancestorKey := formatAncestorKey(childReq.Lineage.AncestorKeys[0])
	if got := len(driver.descendantsByAncestor[ancestorKey]); got != 0 {
		t.Fatalf("expected expired descendant membership cleanup, got %d entries", got)
	}
}

func TestMemoryDriverReleaseWithLineagePreservesOtherDescendants(t *testing.T) {
	driver := NewMemoryDriver()
	firstChildReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child-1",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}
	secondChildReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-2",
		OwnerID:      "worker-b",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child-2",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}

	firstLease, err := driver.AcquireWithLineage(context.Background(), firstChildReq)
	if err != nil {
		t.Fatalf("first child acquire failed: %v", err)
	}
	secondLease, err := driver.AcquireWithLineage(context.Background(), secondChildReq)
	if err != nil {
		t.Fatalf("second child acquire failed: %v", err)
	}

	if err := driver.ReleaseWithLineage(context.Background(), firstLease, firstChildReq.Lineage); err != nil {
		t.Fatalf("first child release failed: %v", err)
	}

	_, err = driver.AcquireWithLineage(context.Background(), backend.LineageAcquireRequest{
		DefinitionID: "order",
		ResourceKey:  "order:123",
		OwnerID:      "worker-c",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    backend.KindParent,
		},
	})
	if !errors.Is(err, backend.ErrOverlapRejected) {
		t.Fatalf("expected remaining child to keep parent blocked, got %v", err)
	}

	if err := driver.ReleaseWithLineage(context.Background(), secondLease, secondChildReq.Lineage); err != nil {
		t.Fatalf("second child release failed: %v", err)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test ./backend/memory -v`
Expected: all tests pass

---

### Task 3: Create backend/memory/assertions.go

**Files:**
- Create: `backend/memory/assertions.go`

- [ ] **Step 1: Create the assertions file**

```go
package memory

import (
	"testing"

	"github.com/tuanuet/lockman/backend"
)

// AssertSingleResourceLease ensures a lease record matches a single key expectation.
func AssertSingleResourceLease(t *testing.T, lease backend.LeaseRecord, defID, ownerID, resourceKey string) {
	t.Helper()

	if lease.DefinitionID != defID {
		t.Fatalf("expected definition %q, got %q", defID, lease.DefinitionID)
	}

	if lease.OwnerID != ownerID {
		t.Fatalf("expected owner %q, got %q", ownerID, lease.OwnerID)
	}

	if len(lease.ResourceKeys) != 1 {
		t.Fatalf("expected 1 resource key, got %d", len(lease.ResourceKeys))
	}

	if lease.ResourceKeys[0] != resourceKey {
		t.Fatalf("expected resource key %q, got %q", resourceKey, lease.ResourceKeys[0])
	}
}
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./backend/memory`
Expected: no errors

---

### Task 4: Commit backend/memory package

- [ ] **Step 1: Commit**

```bash
git add backend/memory/
git commit -m "feat: add public backend/memory in-memory driver for unit testing"
```

---

## Chunk 2: Update all imports and delete old package

### Task 5: Update all import references

**Files to modify** (replace `github.com/tuanuet/lockman/lockkit/testkit` → `github.com/tuanuet/lockman/backend/memory`):

- [ ] **Step 1: Update root-level test files**

```bash
# Use sed for bulk replacement across all Go files
find . -name '*.go' -type f -exec grep -l 'github.com/tuanuet/lockman/lockkit/testkit' {} \;
```

Update each file individually. The full list:
- `client_test.go`
- `client_hold_test.go`
- `client_run_test.go`
- `client_claim_test.go`
- `debug_bridge_test.go`
- `lockkit/runtime/composite_test.go`
- `lockkit/runtime/exclusive_test.go`
- `lockkit/runtime/shutdown_test.go`
- `lockkit/runtime/presence_test.go`
- `lockkit/holds/manager_test.go`
- `lockkit/workers/manager_test.go`
- `lockkit/workers/execute_test.go`
- `lockkit/workers/execute_composite_test.go`
- `benchmarks/benchmark_adoption_helpers_test.go`
- `benchmarks/benchmark_adoption_baseline_test.go`
- `benchmarks/benchmark_layer1_test.go`
- `advanced/composite/api_test.go`
- `advanced/strict/api_test.go`
- `examples/sdk/parent-lock-over-composite/main.go`
- `examples/sdk/shared-aggregate-split-definitions/main.go`
- `examples/core/composite-overlap-reject/main.go`
- `examples/core/sync-composite-lock/main.go`
- `examples/core/parent-lock-over-composite/main.go`
- `examples/core/sync-lock-contention/main.go`
- `examples/core/parent-child-overlap/main.go`
- `examples/core/strict-sync-fencing/main.go`
- `examples/core/sync-single-resource/main.go`
- `examples/core/parent-child-lineage/main.go`
- `examples/core/lease-ttl-expiry/main.go`
- `examples/core/sync-reentrant-reject/main.go`

For each file, change:
```go
"github.com/tuanuet/lockman/lockkit/testkit"
```
to:
```go
"github.com/tuanuet/lockman/backend/memory"
```

And change usage from `testkit.MemoryDriver` / `testkit.AssertSingleResourceLease` / `testkit.NewMemoryDriver()` to `memory.MemoryDriver` / `memory.AssertSingleResourceLease` / `memory.NewMemoryDriver()`.

- [ ] **Step 2: Verify no remaining references**

Run: `grep -r 'lockkit/testkit' --include='*.go' .`
Expected: no results (only `.md` and plan files should remain)

---

### Task 6: Delete lockkit/testkit directory

- [ ] **Step 1: Remove the old directory**

```bash
rm -rf lockkit/testkit
```

- [ ] **Step 2: Verify build**

Run: `go test ./... -run '^$'`
Expected: compiles without errors

---

### Task 7: Update documentation

**Files:**
- Modify: `SKILL.md`

- [ ] **Step 1: Update SKILL.md**

Find the line:
```
| Test support | `lockkit/testkit.MemoryDriver` — in-memory backend for tests | `lockkit/testkit` |
```
Replace with:
```
| Test support | `backend/memory.MemoryDriver` — in-memory backend for tests | `backend/memory` |
```

- [ ] **Step 2: Run full test suite**

Run: `go test ./...`
Expected: all tests pass

- [ ] **Step 3: Run CI parity checks**

Run:
```bash
go test ./...
GOWORK=off go test ./...
go test ./backend/redis/...
go test ./idempotency/redis/...
go test ./guard/postgres/...
go test -tags lockman_examples ./examples/... -run '^$'
```
Expected: all pass

- [ ] **Step 4: Run lint**

Run: `make lint`
Expected: no issues

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "refactor: move testkit to backend/memory, remove lockkit/testkit"
```

---

## Chunk 3: Final verification

### Task 8: Final checks

- [ ] **Step 1: Run benchmarks compile check**

Run: `go test -run '^$' ./benchmarks`
Expected: compiles

- [ ] **Step 2: Verify no stale imports anywhere**

Run: `grep -r 'lockkit/testkit' --include='*.go' .`
Expected: empty

- [ ] **Step 3: Final commit if any stragglers**

```bash
git status
```
