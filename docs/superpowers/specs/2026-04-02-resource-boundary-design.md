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

- `orderBoundary.Bind()` returns a `UseCaseOption` that sets `boundaryID` in the usecase config
- Usecases bound to a boundary use the boundary's ID instead of their own definitionID for lock key construction
- `Bind()` does NOT validate restrictions — it only stores the boundaryID. Restrictions are enforced later in `buildClientPlan()` (see "Restriction Enforcement" below)

#### 3. Force Release (Admin Operation)

Force release is a method on `Client`, not `ResourceBoundary`, since it needs backend access:

```go
err := client.ForceReleaseBoundary(ctx, orderBoundary, "order:123")
```

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

#### Strict Mode

Strict mode (fencing tokens) works with boundaries. All auxiliary keys use the boundary ID:

- Fence counter key: `lockman:lease:fence:bdry_<hash>:<resourceKey>`
- Strict token key: `lockman:lease:strict-token:bdry_<hash>:<resourceKey>`
- Fencing token validation unchanged — only checks ownerID, independent of usecase name

#### Composite Mode

Composite usecases (composite run/claim) do **not** support boundaries. Composite mode acquires multiple locks as a single atomic operation, each with its own definitionID. Sharing boundaries across composite members would complicate atomicity guarantees.

#### Hold Mode

Hold usecases do **not** support boundaries. Hold uses a separate data flow (`DetachedAcquireRequest` / `holds.Manager`) that does not go through the SDK translation layer where boundaryID propagation occurs.

#### Restriction Enforcement

All incompatibility checks are enforced in `buildClientPlan()` in `client_validation.go`, where the usecase kind, composite config, lineage parent, and boundaryID are all accessible:

```go
if useCase.config.boundaryID != "" {
    if useCase.kind == useCaseKindHold {
        return clientPlan{}, fmt.Errorf("lockman: hold use case %q cannot be bound to a boundary", useCase.name)
    }
    if len(useCase.config.composite) > 0 {
        return clientPlan{}, fmt.Errorf("lockman: composite use case %q cannot be bound to a boundary", useCase.name)
    }
    if strings.TrimSpace(useCase.config.lineageParent) != "" {
        return clientPlan{}, fmt.Errorf("lockman: boundary-bound use case %q cannot use lineage mode", useCase.name)
    }
}
```

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

### Boundary Lifecycle

- Boundaries are created at startup via `DefineResourceBoundary()`
- No runtime creation/destruction — boundaries are static after client initialization
- No `Close()` or `Shutdown()` method — boundaries are lightweight (just a name + ID)
- Boundary registry is global (package-level) and cleaned up when the process exits

### Usecase Kind Compatibility

| Usecase Kind | Boundary Support | Notes |
|--------------|-----------------|-------|
| `DefineRun`  | Yes | Full support |
| `DefineHold` | No | Separate data flow (DetachedAcquireRequest); restriction enforced in buildClientPlan |
| `DefineClaim`| Yes | Full support |
| Composite Run/Claim | No | Composite atomicity conflicts with shared boundaries |

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

func (b *ResourceBoundary) Bind() UseCaseOption {
    return func(cfg *useCaseConfig) {
        cfg.boundaryID = b.id
    }
}

func (b *ResourceBoundary) ID() string {
    return b.id
}
```

#### useCaseConfig Change

```go
type useCaseConfig struct {
    // ... existing fields ...
    boundaryID string // boundary ID for shared lock key construction (empty = no boundary)
}
```

#### ForceReleaseBoundary on Client

```go
// client_boundary.go (root lockman package)
func (c *Client) ForceReleaseBoundary(ctx context.Context, boundary *ResourceBoundary, resourceKey string) error {
    // Build all possible auxiliary key forms
    keys := []string{
        c.backend.buildLeaseKey(boundary.id, resourceKey),
        c.backend.buildStrictFenceCounterKey(boundary.id, resourceKey),
        c.backend.buildStrictTokenKey(boundary.id, resourceKey),
    }
    // Unconditionally delete all keys (DEL on non-existent is no-op)
    if err := c.backend.deleteKeys(ctx, keys...); err != nil {
        return fmt.Errorf("lockman: force release boundary: %w", err)
    }
    return nil
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

type claimRequest struct {
    // ... existing fields ...
    boundaryID string
}
func BindClaimRequest(uc UseCase, resourceKey, ownerID, boundaryID string) (*claimRequest, error) {
    // ... pass boundaryID through ...
}

// internal/sdk/translate_run.go
func translateRun(req *runRequest) definitions.SyncLockRequest {
    return definitions.SyncLockRequest{
        // ... existing fields ...
        BoundaryID: req.boundaryID,
    }
}

// internal/sdk/translate_claim.go
func translateClaim(req *claimRequest) definitions.MessageClaimRequest {
    return definitions.MessageClaimRequest{
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

#### client_validation.go Changes

In `client_validation.go`, during request validation, the `boundaryID` from `useCaseCore.config.boundaryID` must be passed through to the SDK layer:

```go
// In validateRunRequest
req.sdkReq, err = sdk.BindRunRequest(
    normalizedUseCase,
    req.resourceKey,
    identity.OwnerID,
    req.useCaseCore.config.boundaryID,  // pass boundaryID through
)

// In validateClaimRequest (parallel change)
req.sdkReq, err = sdk.BindClaimRequest(
    normalizedUseCase,
    req.resourceKey,
    identity.OwnerID,
    req.useCaseCore.config.boundaryID,  // pass boundaryID through
)
```

#### Key Resolution in Runtime (exclusive.go)

In `lockkit/runtime/exclusive.go`, compute an `effectiveID` that overrides the definitionID for all downstream operations (lease key, guard key, active counter):

```go
effectiveID := def.ID
if req.BoundaryID != "" {
    effectiveID = req.BoundaryID
}
```

Pass `effectiveID` consistently to:
- `LeaseContext.DefinitionID` (for observe/logging)
- `guardKey{definitionID: effectiveID}` (for reentrant-acquire protection)
- `m.activeCounter(effectiveID)` (for active lock counters)

Pass `req.BoundaryID` to backend requests:

```go
// Standard acquire
lease, err = m.acquireLease(ctx, def, acquirePlan, req.Ownership.OwnerID)
// AcquireRequest{... BoundaryID: req.BoundaryID ...}

// Strict acquire
fenced, err = strictDriver.AcquireStrict(ctx, backend.StrictAcquireRequest{
    ...,
    BoundaryID: req.BoundaryID,
})
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
| `boundary.go` | New: ResourceBoundary type, DefineResourceBoundary, Bind, ID, global registry |
| `client_boundary.go` | New: Client.ForceReleaseBoundary method |
| `binding.go` | Modify: Add boundaryID to useCaseConfig |
| `client_validation.go` | Modify: Pass boundaryID to sdk.BindRunRequest/BindClaimRequest; add boundary restriction checks in buildClientPlan |
| `backend/contracts.go` | Modify: Add BoundaryID to AcquireRequest, StrictAcquireRequest |
| `backend/redis/driver.go` | Modify: resolveKey method for boundary ID; add deleteKeys helper |
| `internal/sdk/request.go` | Modify: Add boundaryID to runRequest/claimRequest; pass through BindRunRequest/BindClaimRequest |
| `internal/sdk/translate_run.go` | Modify: Set BoundaryID on SyncLockRequest |
| `internal/sdk/translate_claim.go` | Modify: Set BoundaryID on SyncLockRequest |
| `lockkit/definitions/ownership.go` | Modify: Add BoundaryID to SyncLockRequest |
| `lockkit/runtime/exclusive.go` | Modify: effectiveID computation; pass BoundaryID to backend requests; use effectiveID in guard keys and active counters |
| `boundary_test.go` | New: unit tests for boundary API |
| `examples/core/boundary-usage/main.go` | New: example usage |

### Testing Strategy

1. **Unit tests:**
   - Boundary define/bind validation
   - Key construction with boundary ID
   - Global registry: duplicate name panics
   - BoundaryID data flow through BindRunRequest → translateRun → SyncLockRequest
   - Define-time rejection of lineage + boundary (in buildClientPlan)
   - Define-time rejection of composite + boundary (in buildClientPlan)
   - Define-time rejection of Hold + boundary (in buildClientPlan)

2. **Integration tests:**
   - Two usecases in same boundary → mutual exclusion (one acquires, other fails)
   - Force release behavior (cleans lease + auxiliary keys, idempotent on non-existent lock)
   - Strict mode + boundary interaction (fencing tokens work correctly)

3. **Backward compatibility tests:**
   - Non-boundary usecases unchanged (plain ownerID in lease key)
   - Mixed environment (some boundary, some not) coexist in same Redis instance

### Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Boundary name collision across teams | Document that boundary names should be globally unique within shared Redis; consider namespace prefix if needed |
| Lineage incompatibility | Enforced in buildClientPlan; documented restriction; future enhancement tracked |
| Composite incompatibility | Enforced in buildClientPlan; documented restriction |
| Hold incompatibility | Enforced in buildClientPlan; documented restriction |
