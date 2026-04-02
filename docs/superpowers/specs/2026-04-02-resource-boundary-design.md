# Resource Boundary Design

## Problem Statement

Currently, lock keys are constructed from `DefinitionID:resourceKey`, where `DefinitionID` is a hash derived from `useCaseName + kind`. This means two different usecases (e.g., `order.process` and `order.cancel`) cannot mutually exclude on the same resource (e.g., `order:123`) — they produce two different Redis keys.

**Requirements:** Allow multiple usecases to share a lock boundary so that:
1. Mutual exclusion on the same resource across usecases
2. Cross-usecase visibility (query "which usecase holds the lock")
3. Explicit API — declared clearly at define time

## Solution: ResourceBoundary

### Core Concept

`ResourceBoundary` is an abstraction independent of usecases, representing a "lock domain". Multiple usecases bind to the same boundary → shared lock key + cross-usecase visibility.

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

#### 3. Query Active Locks

```go
activeLocks, err := orderBoundary.ActiveLocks(ctx)
// Returns: []BoundaryLockInfo{
//   {ResourceKey: "order:123", UsecaseName: "order.process", OwnerID: "...", AcquiredAt: ..., ExpiresAt: ..., Mode: "standard"},
// }
```

- Uses a maintained index key (Redis Set) for O(1) discovery, not SCAN
- Index key format: `lockman:lease:bdry_idx:<boundaryID_hash>`
- Index is maintained atomically within acquire/release Lua scripts
- Index entries are cleaned up on release and TTL expiry (via TTL on lease key, index cleaned on release)
- Returns cursor-based pagination: `ActiveLocks(ctx, WithLimit(100), WithCursor(cursor))`

#### 4. Force Release (Admin Operation)

```go
err := orderBoundary.Release(ctx, "order:123")
```

- Releases any lock within the boundary without owner validation
- Cleans up all auxiliary keys: lease, metadata, fence counter, strict token, lineage, and index entry
- Used for admin/ops scenarios (cleanup, recovery)

### Key Construction

#### Before (current)
```
lockman:lease:<definitionID>:<resourceKey>
```

#### After (with boundary)
```
Lease key:     lockman:lease:bdry_<boundaryID_hash>:<resourceKey>
Metadata key:  lockman:lease:bdry_<boundaryID_hash>:<resourceKey>:meta
Index key:     lockman:lease:bdry_idx:<boundaryID_hash>  (Redis Set of resourceKeys)
```

- `boundaryID` = stable hash from boundary name (FNV-64a, format: `bdry_<hex>`)
- Non-boundary usecases remain unchanged — fully backward compatible
- **Critical design decision:** Metadata is stored in a separate key, NOT in the lease value. This preserves all existing Lua scripts unchanged, since they perform direct string equality checks on the lease value (ownerID). Changing the lease value format would break every Lua script.

### Lease Metadata Format (Separate Key)

The metadata key stores JSON with visibility information:

```json
{"o":"owner123","u":"order.process","a":"2026-04-02T10:00:00Z","e":"2026-04-02T10:00:30Z","m":"standard"}
```

Fields:
- `o` = ownerID
- `u` = usecaseName (the usecase that acquired the lock)
- `a` = acquiredAt timestamp (RFC3339)
- `e` = expiresAt timestamp (acquiredAt + leaseTTL, RFC3339)
- `m` = mode ("standard", "strict", "lineage")

**Backward compatibility:** Non-boundary usecases continue using plain ownerID in the lease key. No migration needed for existing deployments. Boundary usecases always write both lease + metadata keys atomically.

### Integration with Existing Features

#### Lineage Mode (Out of Scope for v1)

Boundary + lineage integration is **out of scope for v1**. Lineage is currently built around usecase-to-usecase parent-child relationships (via `lineageParent` field in `useCaseConfig`), which does not map cleanly to boundary semantics. This will be evaluated as a future enhancement.

Boundary-bound usecases can still use lineage with their own definitionID — the boundary only affects the lease key, not lineage keys. Lineage keys continue using the usecase's own definitionID.

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
    ErrBoundaryCollision  = errors.New("lockman: boundary already defined")
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
// Force release from boundary object → always succeeds (admin operation)
```

#### Stale Visibility Data
```go
type BoundaryLockInfo struct {
    ResourceKey string
    UsecaseName string
    OwnerID     string
    AcquiredAt  time.Time
    ExpiresAt   time.Time
    Mode        string    // "standard", "strict", "lineage"
    // Out of scope for v1: FencingToken, Lineage metadata
}
```

`ActiveLocks()` may return stale data (lock expired between query and return) → callers should validate `ExpiresAt.After(time.Now())`.

### Boundary Lifecycle

- Boundaries are created at startup via `DefineResourceBoundary()`
- No runtime creation/destruction — boundaries are static after client initialization
- No `Close()` or `Shutdown()` method — boundaries are lightweight (just a name + ID)
- Boundary registry is cleaned up when the lockman client shuts down

### Usecase Kind Compatibility

| Usecase Kind | Boundary Support | Notes |
|--------------|-----------------|-------|
| `DefineRun`  | Yes | Full support |
| `DefineHold` | Yes | Metadata includes token info; `ActiveLocks()` sees Hold leases |
| `DefineClaim`| Yes | Same as Run |

### Observability

Boundary-specific metrics:

```
# Prometheus-style metrics
lockman_boundary_locks_active{boundary="order"} → gauge
lockman_boundary_acquires_total{boundary="order", usecase="order.process"} → counter
lockman_boundary_contentions_total{boundary="order"} → counter
```

**Cardinality controls:**
- `boundary` label value uses the boundary name (not ID)
- Boundary names should be bounded (do not generate dynamic boundary names per-request)
- `usecase` label is limited to usecases bound to the boundary (bounded set)
- If cardinality becomes a concern, add a config option to disable per-usecase metrics

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

#### Key Resolution in Runtime

When acquiring a lease, the runtime checks `boundaryID` in `useCaseConfig`:
- If `boundaryID` is set → use boundary ID for key construction
- If `boundaryID` is empty → use definitionID (current behavior)

This check happens in `lockkit/runtime/exclusive.go` before calling the backend.

#### Atomic Metadata + Index Writes

Acquire Lua script (boundary mode) writes 3 keys atomically:
1. Lease key: `SET leaseKey ownerID PX ttl NX`
2. Metadata key: `SET metaKey metadataJSON PX ttl`
3. Index key: `SADD indexKey resourceKey`

Release Lua script (boundary mode) deletes 3 keys atomically:
1. Lease key: `DEL leaseKey`
2. Metadata key: `DEL metaKey`
3. Index key: `SREM indexKey resourceKey`

Force Release (boundary admin operation) deletes all keys including auxiliary (fence, strict-token, lineage).

### File Changes (Expected)

| File | Change |
|------|--------|
| `boundary.go` | New: ResourceBoundary type, DefineResourceBoundary, Bind, ActiveLocks, Release |
| `backend/contracts.go` | New: BoundaryLockInfo struct, BoundaryDriver interface additions |
| `backend/redis/driver.go` | Modify: buildLeaseKey support boundary ID, new buildMetadataKey/buildIndexKey methods |
| `backend/redis/scripts.go` | New: boundaryAcquireScript, boundaryReleaseScript, boundaryForceReleaseScript |
| `internal/sdk/usecase.go` | No changes needed (boundary ID resolved at runtime layer) |
| `lockkit/runtime/exclusive.go` | Modify: resolve boundary ID before acquire, pass to backend |
| `metrics.go` | New: boundary-specific metrics registration |
| `errors.go` | New: ErrNotBoundToBoundary sentinel |
| `boundary_test.go` | New: unit tests for boundary API |
| `examples/core/boundary-usage/main.go` | New: example usage |

### Testing Strategy

1. **Unit tests:**
   - Boundary define/bind validation
   - Key construction with boundary ID
   - Metadata encode/decode
   - Index key maintenance (SADD/SREM)

2. **Integration tests:**
   - Two usecases in same boundary → mutual exclusion (one acquires, other fails)
   - ActiveLocks() accuracy (returns correct usecase name, owner, timestamps)
   - Force release behavior (cleans all keys including auxiliary)
   - Strict mode + boundary interaction (fencing tokens work correctly)
   - Hold usecase + boundary (metadata includes token info)

3. **Backward compatibility tests:**
   - Non-boundary usecases unchanged (plain ownerID in lease key)
   - Mixed environment (some boundary, some not) coexist in same Redis instance

### Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Additional Redis key per lock (metadata + index) | Benchmark memory impact; metadata key has same TTL as lease; index is bounded by active locks |
| Lua script complexity (3-key atomic ops) | Scripts remain simple; 3-key ops are well within Redis limits |
| Boundary name collision across teams | Document that boundary names should be globally unique within shared Redis; consider namespace prefix if needed |
| Index key drift (orphaned entries) | Force release cleans index; add periodic cleanup job as future enhancement |
| Metrics cardinality explosion | Document bounded boundary names; add config to disable per-usecase metrics |
