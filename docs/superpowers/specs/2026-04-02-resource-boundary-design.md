# Resource Boundary Design

## Problem Statement

Currently, lock keys are constructed from `DefinitionID:resourceKey`, where `DefinitionID` is a hash derived from `useCaseName + kind`. This means two different usecases (e.g., `order.process` and `order.cancel`) cannot mutually exclude on the same resource (e.g., `order:123`) — they produce two different Redis keys.

**Requirements:** Allow multiple usecases to share a lock boundary so that:
1. Mutual exclusion on the same resource across usecases
2. Explicit API — declared clearly at define time
3. Visibility is handled by existing observe/logging, not stored in Redis

## Solution: ResourceBoundary

### Core Concept

`ResourceBoundary` is an abstraction independent of usecases, representing a "lock domain". Multiple usecases bind to the same boundary → shared lock key.

### API Design

#### 1. Define Boundary

```go
orderBoundary := lockman.DefineResourceBoundary("order")
```

- Boundary name must be unique within a client instance
- Panics on duplicate name: `"lockman: boundary 'order' already defined"`
- Boundary names are intended to be globally unique across services that share the same Redis instance. Two services defining `DefineResourceBoundary("order")` will produce the same `bdry_<hash>` and share locks — this is intentional for cross-service coordination.

#### 2. Bind Usecases to Boundary

```go
processUC := lockman.DefineRun("order.process", orderBoundary.Bind())
cancelUC := lockman.DefineRun("order.cancel", orderBoundary.Bind())
```

- `orderBoundary.Bind()` returns a `UseCaseOption`
- Usecases bound to a boundary use the boundary's ID instead of their own definitionID for lock key construction
- **Restriction:** Usecases bound to a boundary cannot also use lineage mode

#### 3. Force Release (Admin Operation)

```go
err := orderBoundary.Release(ctx, "order:123")
```

- Releases any lock within the boundary without owner validation
- Idempotent: returns `nil` if the lock does not exist
- Cleans up lease key plus any auxiliary keys (fence counter, strict token)
- Used for admin/ops scenarios (cleanup, recovery)

### Key Construction

#### Before (current)
```
lockman:lease:<definitionID>:<resourceKey>
```

#### After (with boundary)
```
lockman:lease:bdry_<boundaryID_hash>:<resourceKey>
```

- `boundaryID` = stable hash from boundary name (FNV-64a, format: `bdry_<hex>`)
- Hash construction: `FNV-64a("b" + boundaryName)` — the `"b"` prefix distinguishes boundary hashes from usecase hashes (which use kind delimiters like `"r"`, `"c"`, `"h"`)
- Non-boundary usecases remain unchanged — fully backward compatible
- Lease value remains plain ownerID string — no changes to Lua scripts

### Integration with Existing Features

#### Lineage Incompatibility

Boundary-bound usecases **cannot** use lineage mode. Lineage overlap checks depend on verifying ancestor lease keys, but boundary-based lease keys use a different ID (`bdry_<hash>`) than the usecase's definitionID. Mixing the two would cause lineage to check the wrong key, silently breaking protection.

**Enforcement:** If a usecase is bound to a boundary AND has `lineageParent` set, panic at define time: `"lockman: boundary-bound usecase cannot use lineage mode"`.

#### Strict Mode

Strict mode (fencing tokens) works with boundaries. All auxiliary keys use the boundary ID:

- Fence counter key: `lockman:lease:fence:bdry_<hash>:<resourceKey>`
- Strict token key: `lockman:lease:strict-token:bdry_<hash>:<resourceKey>`
- Fencing token validation unchanged — only checks ownerID, independent of usecase name

### Error Handling

#### Sentinel Errors

```go
var (
    ErrNotBoundToBoundary = errors.New("lockman: use case is not bound to the requested boundary")
)
```

#### Boundary Collision
```go
DefineResourceBoundary("order") // ok
DefineResourceBoundary("order") // panics: "lockman: boundary 'order' already defined"
```

#### Cross-Boundary Release
```go
// Release from a usecase not bound to the boundary → ErrNotBoundToBoundary
// Force release from boundary object → always succeeds (idempotent admin operation)
```

### Boundary Lifecycle

- Boundaries are created at startup via `DefineResourceBoundary()`
- No runtime creation/destruction — boundaries are static after client initialization
- No `Close()` or `Shutdown()` method — boundaries are lightweight (just a name + ID)
- Boundary registry is cleaned up when the lockman client shuts down

### Usecase Kind Compatibility

| Usecase Kind | Boundary Support | Notes |
|--------------|-----------------|-------|
| `DefineRun`  | Yes | Full support |
| `DefineHold` | Yes | Full support |
| `DefineClaim`| Yes | Full support |

### Implementation Details

#### useCaseConfig Change

```go
type useCaseConfig struct {
    // ... existing fields ...
    boundaryID string // boundary ID for shared lock key construction (empty = no boundary)
}
```

`Bind()` implementation:
```go
func (b *ResourceBoundary) Bind() UseCaseOption {
    return func(cfg *useCaseConfig) {
        cfg.boundaryID = b.id
    }
}
```

#### BoundaryID Data Flow Through the Translation Chain

The boundaryID must flow from the define-time config through to the runtime layer. Here is the complete data flow:

1. **Define time:** `DefineRun("order.process", orderBoundary.Bind())` → sets `useCaseConfig.boundaryID`
2. **Request time:** `uc.Run(ctx, req, fn)` → `RunRequest` carries `useCaseCore.config.boundaryID`
3. **Client layer:** `client_run.go` reads `RunRequest.BoundaryID` and sets `definitions.SyncLockRequest.BoundaryID`
4. **Runtime execution:** `lockkit/runtime/exclusive.go` reads `SyncLockRequest.BoundaryID` and passes it to backend
5. **Backend acquire:** `backend.AcquireRequest.BoundaryID` → driver uses boundaryID if set, otherwise uses DefinitionID

**Required struct changes:**

```go
// lockkit/definitions/ownership.go
type SyncLockRequest struct {
    // ... existing fields ...
    BoundaryID string // empty = use DefinitionID
}

// backend/contracts.go
type AcquireRequest struct {
    // ... existing fields ...
    BoundaryID string // empty = use DefinitionID for key construction
}

// backend/contracts.go
type StrictAcquireRequest struct {
    // ... existing fields ...
    BoundaryID string
}

// backend/contracts.go
type LineageAcquireRequest struct {
    // ... existing fields ...
    BoundaryID string
}
```

#### Key Resolution in Runtime (exclusive.go)

In `lockkit/runtime/exclusive.go`, the `acquireLease` function must pass `SyncLockRequest.BoundaryID` through to all three backend request types:

```go
// Standard acquire (existing code path, add BoundaryID)
lease, err = m.acquireLease(ctx, def, acquirePlan, req.Ownership.OwnerID)
// → backend.AcquireRequest{... BoundaryID: req.BoundaryID ...}

// Strict acquire (existing code path, add BoundaryID)
fenced, err = strictDriver.AcquireStrict(ctx, backend.StrictAcquireRequest{
    ...,
    BoundaryID: req.BoundaryID,
})

// Lineage acquire — NOT supported with boundary (enforced at define time)
```

#### Key Resolution in Backend Driver

```go
func (d *Driver) resolveKey(definitionID, boundaryID, resourceKey string) string {
    keyID := definitionID
    if boundaryID != "" {
        keyID = boundaryID
    }
    return d.keyPrefix + ":" + d.encodeDefinitionID(keyID) + ":" + encodeSegment(resourceKey)
}
```

### File Changes (Expected)

| File | Change |
|------|--------|
| `boundary.go` | New: ResourceBoundary type, DefineResourceBoundary, Bind, Release |
| `backend/contracts.go` | Modify: Add BoundaryID to AcquireRequest, StrictAcquireRequest, LineageAcquireRequest |
| `backend/redis/driver.go` | Modify: resolveKey method for boundary ID |
| `backend/redis/scripts.go` | No changes (boundary force release uses existing release pattern) |
| `lockkit/definitions/ownership.go` | Modify: Add BoundaryID to SyncLockRequest |
| `lockkit/runtime/exclusive.go` | Modify: pass BoundaryID from SyncLockRequest to backend AcquireRequest, StrictAcquireRequest |
| `errors.go` | New: ErrNotBoundToBoundary sentinel |
| `boundary_test.go` | New: unit tests for boundary API |
| `examples/core/boundary-usage/main.go` | New: example usage |

### Testing Strategy

1. **Unit tests:**
   - Boundary define/bind validation
   - Key construction with boundary ID
   - BoundaryID data flow through SyncLockRequest → AcquireRequest
   - Define-time rejection of lineage + boundary

2. **Integration tests:**
   - Two usecases in same boundary → mutual exclusion (one acquires, other fails)
   - Force release behavior (cleans lease + auxiliary keys, idempotent on non-existent lock)
   - Strict mode + boundary interaction (fencing tokens work correctly)
   - Hold usecase + boundary (acquire/release work correctly)

3. **Backward compatibility tests:**
   - Non-boundary usecases unchanged (plain ownerID in lease key)
   - Mixed environment (some boundary, some not) coexist in same Redis instance

### Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Boundary name collision across teams | Document that boundary names should be globally unique within shared Redis; consider namespace prefix if needed |
| Lineage incompatibility | Enforced at define time; documented restriction; future enhancement tracked |
