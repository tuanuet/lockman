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

- `DefineResourceBoundary` is a **package-level function** in the root `lockman` package
- Boundary is owned by a **global registry** (package-level), not by any specific `Client`
- Boundary name must be unique across all boundaries in the process
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
- **Restriction:** Hold usecases cannot be bound to a boundary (different data flow)

#### 3. Force Release (Admin Operation)

`Release()` is a method on `ResourceBoundary` that takes an explicit `*Client` parameter for backend access:

```go
err := orderBoundary.Release(ctx, client, "order:123")
```

- `ResourceBoundary` does NOT hold an internal `*Client` reference — the client is passed explicitly at call time
- Releases any lock within the boundary without owner validation
- Idempotent: returns `nil` if the lock does not exist
- Cleans up lease key plus any auxiliary keys (fence counter, strict token) — uses `DEL` on all possible key forms unconditionally (Redis `DEL` on non-existent keys is a no-op)
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

#### Composite Mode

Composite usecases (composite run/claim) do **not** support boundaries. Composite mode acquires multiple locks as a single atomic operation, each with its own definitionID. Sharing boundaries across composite members would complicate atomicity guarantees.

**Enforcement:** If a composite usecase is bound to a boundary, panic at define time.

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
- Boundary registry is global (package-level) and cleaned up when the process exits

### Usecase Kind Compatibility

| Usecase Kind | Boundary Support | Notes |
|--------------|-----------------|-------|
| `DefineRun`  | Yes | Full support |
| `DefineHold` | No | Hold uses a separate data flow (DetachedAcquireRequest, holds.Manager); restriction enforced at define time |
| `DefineClaim`| Yes | Full support |
| Composite Run/Claim | No | Enforced at define time; composite atomicity conflicts with shared boundaries |

### Implementation Details

#### Boundary Registry (Global, Package-Level)

```go
// boundary.go (root lockman package)
var boundaryRegistry = &sync.Map{} // map[string]*ResourceBoundary

type ResourceBoundary struct {
    name string
    id   string // bdry_<hex>
}

func DefineResourceBoundary(name string) *ResourceBoundary {
    trimmed := strings.TrimSpace(name)
    if trimmed == "" {
        panic("lockman: boundary name must not be empty")
    }
    id := stableBoundaryID(trimmed)
    if _, loaded := boundaryRegistry.LoadOrStore(trimmed, &ResourceBoundary{name: trimmed, id: id}); loaded {
        panic(fmt.Sprintf("lockman: boundary %q already defined", trimmed))
    }
    return &ResourceBoundary{name: trimmed, id: id}
}

func stableBoundaryID(name string) string {
    hash := fnv.New64a()
    _, _ = hash.Write([]byte{'b'})
    _, _ = hash.Write([]byte(name))
    return "bdry_" + toHex(hash.Sum64())
}
```

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
        if cfg.kind == useCaseKindHold {
            panic("lockman: Hold usecases cannot be bound to a boundary")
        }
        cfg.boundaryID = b.id
    }
}
```

#### BoundaryID Data Flow Through the Translation Chain

The boundaryID must flow from the define-time config through to the runtime layer. Here is the complete data flow:

1. **Define time:** `DefineRun("order.process", orderBoundary.Bind())` → sets `useCaseConfig.boundaryID` in `binding.go`
2. **Request time:** `uc.Run(ctx, req, fn)` → `RunRequest` carries `useCaseCore` which holds `config.boundaryID`
3. **Validation layer:** `client_validation.go` reads `useCaseCore.config.boundaryID` and passes it through `sdk.BindRunRequest()`
4. **SDK layer:** `internal/sdk/request.go` — `runRequest` struct receives `boundaryID`; `sdk.BindRunRequest()` passes it through; `sdk.translateRun()` sets `definitions.SyncLockRequest.BoundaryID`
5. **Runtime execution:** `lockkit/runtime/exclusive.go` reads `SyncLockRequest.BoundaryID` and passes it to backend
6. **Backend acquire:** `backend.AcquireRequest.BoundaryID` → driver uses boundaryID if set, otherwise uses DefinitionID

**Required struct changes:**

```go
// internal/sdk/request.go
type runRequest struct {
    // ... existing fields ...
    boundaryID string
}
func BindRunRequest(uc UseCase, resourceKey, ownerID, boundaryID string) (*runRequest, error) {
    // ... pass boundaryID through ...
}

// internal/sdk/translate_run.go
func translateRun(req *runRequest) definitions.SyncLockRequest {
    return definitions.SyncLockRequest{
        // ... existing fields ...
        BoundaryID: req.boundaryID,
    }
}

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
```

Note: `LineageAcquireRequest` does NOT need `BoundaryID` — lineage is incompatible with boundaries.

#### client_validation.go Changes

In `client_validation.go`, during request validation, the `boundaryID` from `useCaseCore.config.boundaryID` must be passed through to the SDK layer:

```go
// In validateRunRequest or similar
req.sdkReq, err = sdk.BindRunRequest(
    normalizedUseCase,
    req.resourceKey,
    identity.OwnerID,
    req.useCaseCore.config.boundaryID,  // pass boundaryID through
)
```

#### Key Resolution in Runtime (exclusive.go)

In `lockkit/runtime/exclusive.go`, the `acquireLease` function passes `SyncLockRequest.BoundaryID` to backend requests:

```go
// Standard acquire — in buildAcquirePlan or acquireLease
lease, err = m.acquireLease(ctx, def, acquirePlan, req.Ownership.OwnerID)
// AcquireRequest{... BoundaryID: req.BoundaryID ...}

// Strict acquire
fenced, err = strictDriver.AcquireStrict(ctx, backend.StrictAcquireRequest{
    ...,
    BoundaryID: req.BoundaryID,
})
```

Also update `LeaseContext.DefinitionID` in `exclusive.go` — when `req.BoundaryID` is not empty, override the definitionID used throughout the execution:

```go
effectiveID := def.ID
if req.BoundaryID != "" {
    effectiveID = req.BoundaryID
}
leaseCtx := LeaseContext{DefinitionID: effectiveID, ...}
```

This ensures observe/logging downstream reflects the actual key used, not the usecase's own definitionID.

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

#### Force Release Script

Force release deletes all possible auxiliary keys unconditionally (Redis `DEL` on non-existent keys is a no-op):
1. Lease key: `DEL lockman:lease:bdry_<hash>:<resourceKey>`
2. Fence counter: `DEL lockman:lease:fence:bdry_<hash>:<resourceKey>`
3. Strict token: `DEL lockman:lease:strict-token:bdry_<hash>:<resourceKey>`

No mode detection needed.

### File Changes (Expected)

| File | Change |
|------|--------|
| `boundary.go` | New: ResourceBoundary type, DefineResourceBoundary, Bind, Release, global boundary registry |
| `binding.go` | Modify: Add boundaryID to useCaseConfig |
| `backend/contracts.go` | Modify: Add BoundaryID to AcquireRequest, StrictAcquireRequest |
| `backend/redis/driver.go` | Modify: resolveKey method for boundary ID |
| `internal/sdk/request.go` | Modify: Add boundaryID to runRequest/claimRequest; pass through BindRunRequest/BindClaimRequest |
| `internal/sdk/translate_run.go` | Modify: Set BoundaryID on SyncLockRequest |
| `internal/sdk/translate_claim.go` | Modify: Set BoundaryID on SyncLockRequest |
| `lockkit/definitions/ownership.go` | Modify: Add BoundaryID to SyncLockRequest |
| `lockkit/runtime/exclusive.go` | Modify: pass BoundaryID to backend requests; set LeaseContext.DefinitionID to boundaryID when present |
| `client_validation.go` | Modify: pass useCaseCore.config.boundaryID to sdk.BindRunRequest |
| `errors.go` | New: ErrNotBoundToBoundary sentinel |
| `boundary_test.go` | New: unit tests for boundary API |
| `examples/core/boundary-usage/main.go` | New: example usage |

### Testing Strategy

1. **Unit tests:**
   - Boundary define/bind validation
   - Key construction with boundary ID
   - Global registry: duplicate name panics
   - BoundaryID data flow through BindRunRequest → translateRun → SyncLockRequest
   - Define-time rejection of lineage + boundary
   - Define-time rejection of composite + boundary

2. **Integration tests:**
   - Two usecases in same boundary → mutual exclusion (one acquires, other fails)
   - Force release behavior (cleans lease + auxiliary keys, idempotent on non-existent lock)
   - Strict mode + boundary interaction (fencing tokens work correctly)
   - Hold usecase + boundary → panic at define time (Hold not supported)

3. **Backward compatibility tests:**
   - Non-boundary usecases unchanged (plain ownerID in lease key)
   - Mixed environment (some boundary, some not) coexist in same Redis instance

### Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Boundary name collision across teams | Document that boundary names should be globally unique within shared Redis; consider namespace prefix if needed |
| Lineage incompatibility | Enforced at define time; documented restriction; future enhancement tracked |
| Composite incompatibility | Enforced at define time; documented restriction |
