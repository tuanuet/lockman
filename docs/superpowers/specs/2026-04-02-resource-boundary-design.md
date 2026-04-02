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
- **Restriction:** Usecases bound to a boundary cannot also use lineage mode (see "Lineage Incompatibility" below)

#### 3. Query Active Locks

```go
activeLocks, nextCursor, err := orderBoundary.ActiveLocks(ctx, lockman.WithLimit(100), lockman.WithCursor(cursor))
// Returns: ([]BoundaryLockInfo, nextCursor string, error)
```

- Uses `SSCAN` on the index key for cursor-based iteration
- Index key format: `lockman:lease:bdry_idx:<boundaryID_hash>` (Redis Set of resourceKeys)
- Index is maintained atomically within acquire/release Lua scripts
- Index entries may become orphaned on TTL expiry (see "Index Drift Handling" below)
- Each returned `BoundaryLockInfo` includes `ExpiresAt` for caller-side staleness validation

#### 4. Force Release (Admin Operation)

```go
err := orderBoundary.Release(ctx, "order:123")
```

- Releases any lock within the boundary without owner validation
- Idempotent: returns `nil` if the lock does not exist (no-op for non-existent locks)
- Cleans up all auxiliary keys: lease, metadata, fence counter, strict token, and index entry
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
- Hash construction: `FNV-64a("b" + boundaryName)` — the `"b"` prefix distinguishes boundary hashes from usecase hashes (which use kind delimiters like `"r"`, `"c"`, `"h"`)
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
- `m` = mode ("standard", "strict")

**Encoding/decoding responsibility:** The SDK layer handles JSON encode/decode. The backend layer (`backend/redis/`) treats metadata as an opaque string and only manages TTL. This keeps the backend agnostic of metadata schema.

**Extensibility policy:** New fields are added with new single-letter keys. Readers must ignore unknown fields. Writers must not remove fields from existing keys — only add new ones. This ensures backward-compatible evolution.

**Backward compatibility:** Non-boundary usecases continue using plain ownerID in the lease key. No migration needed for existing deployments. Boundary usecases always write both lease + metadata keys atomically.

### Integration with Existing Features

#### Lineage Incompatibility

Boundary-bound usecases **cannot** use lineage mode. Lineage overlap checks depend on verifying ancestor lease keys, but boundary-based lease keys use a different ID (`bdry_<hash>`) than the usecase's definitionID. Mixing the two would cause lineage to check the wrong key, silently breaking protection.

**Enforcement:** If a usecase is bound to a boundary AND has `lineageParent` set, panic at define time: `"lockman: boundary-bound usecase cannot use lineage mode"`.

This restriction will be evaluated for removal in a future version if lineage key resolution is updated to support boundary-aware lookups.

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
// Force release from boundary object → always succeeds (idempotent admin operation)
```

#### Stale Visibility Data

```go
// Defined in backend/contracts.go
type BoundaryLockInfo struct {
    ResourceKey string
    UsecaseName string
    OwnerID     string
    AcquiredAt  time.Time
    ExpiresAt   time.Time
    Mode        string    // "standard", "strict"
}
```

`ActiveLocks()` may return stale data (lock expired between query and return) → callers should validate `ExpiresAt.After(time.Now())`.

#### Index Drift Handling

Index entries can become orphaned when a lease key expires via TTL (Redis server-side event — no script runs on expiry). `ActiveLocks()` handles this by:

1. Using `SSCAN` to iterate index members
2. For each resourceKey, checking if the lease key still exists (`EXISTS leaseKey`)
3. If lease key is missing, removing the orphaned entry from index (`SREM indexKey resourceKey`) and skipping it
4. Returning only entries with valid lease keys

This ensures `ActiveLocks()` self-heals orphaned index entries at read time, without requiring a separate cleanup job.

### Boundary Lifecycle

- Boundaries are created at startup via `DefineResourceBoundary()`
- No runtime creation/destruction — boundaries are static after client initialization
- No `Close()` or `Shutdown()` method — boundaries are lightweight (just a name + ID)
- Boundary registry is cleaned up when the lockman client shuts down

### Usecase Kind Compatibility

| Usecase Kind | Boundary Support | Notes |
|--------------|-----------------|-------|
| `DefineRun`  | Yes | Full support |
| `DefineHold` | Yes | Metadata uses same JSON schema; `ActiveLocks()` sees Hold leases |
| `DefineClaim`| Yes | Same as Run |

### Observability

Boundary-specific metrics integrate with the existing `observe` package and OTel bridge pattern:

```
# Prometheus-style metrics
lockman_boundary_locks_active{boundary="order"} → gauge
lockman_boundary_acquires_total{boundary="order", usecase="order.process"} → counter
lockman_boundary_contentions_total{boundary="order"} → counter
```

**Integration approach:** Add `BoundaryID string` field to the existing `observe.Event` struct. Boundary metrics reuse the existing event bridge — no new `EventKind` values needed. A new `observe/boundary.go` file provides helper functions for building boundary-specific labels from the existing `Event` type.

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

#### Atomic Metadata + Index Writes

Acquire Lua script (boundary mode) writes 3 keys atomically:
1. Lease key: `SET leaseKey ownerID PX ttl NX`
2. Metadata key: `SET metaKey metadataJSON PX ttl`
3. Index key: `SADD indexKey resourceKey`

Release Lua script (boundary mode) deletes 3 keys atomically:
1. Lease key: `DEL leaseKey`
2. Metadata key: `DEL metaKey`
3. Index key: `SREM indexKey resourceKey`

#### Renew Script (Boundary Mode)

Standard renew only refreshes the lease key TTL via `PEXPIRE`. Boundary mode must also refresh the metadata key TTL to keep visibility data alive:

```lua
-- boundaryRenewScript
local leaseExists = redis.call("EXISTS", KEYS[1])
if leaseExists == 0 then
    return 0
end
redis.call("PEXPIRE", KEYS[1], ARGV[2])
redis.call("PEXPIRE", KEYS[2], ARGV[2])  -- metadata key
return 1
```

Arguments: `KEYS[1]` = lease key, `KEYS[2]` = metadata key, `ARGV[1]` = ownerID (for validation), `ARGV[2]` = TTL in ms.

The index entry does not need TTL refresh — it is self-healed by `ActiveLocks()` via `EXISTS` check.

#### Force Release Script

Force release deletes all possible auxiliary keys unconditionally (Redis `DEL` on non-existent keys is a no-op):
1. Lease key: `DEL lockman:lease:bdry_<hash>:<resourceKey>`
2. Metadata key: `DEL lockman:lease:bdry_<hash>:<resourceKey>:meta`
3. Fence counter: `DEL lockman:lease:fence:bdry_<hash>:<resourceKey>`
4. Strict token: `DEL lockman:lease:strict-token:bdry_<hash>:<resourceKey>`
5. Index entry: `SREM lockman:lease:bdry_idx:<hash> <resourceKey>`

No mode detection needed — `DEL` on non-existent keys is harmless.

### File Changes (Expected)

| File | Change |
|------|--------|
| `boundary.go` | New: ResourceBoundary type, DefineResourceBoundary, Bind, ActiveLocks, Release |
| `backend/contracts.go` | Modify: Add BoundaryID to AcquireRequest, StrictAcquireRequest, LineageAcquireRequest; add BoundaryLockInfo struct |
| `backend/redis/driver.go` | Modify: resolveKey method for boundary ID, new buildMetadataKey/buildIndexKey methods |
| `backend/redis/scripts.go` | New: boundaryAcquireScript, boundaryReleaseScript, boundaryRenewScript, boundaryForceReleaseScript |
| `lockkit/definitions/ownership.go` | Modify: Add BoundaryID to SyncLockRequest |
| `lockkit/runtime/exclusive.go` | Modify: pass BoundaryID from SyncLockRequest to backend AcquireRequest, StrictAcquireRequest |
| `observe/event.go` | Modify: Add BoundaryID field to Event struct |
| `observe/boundary.go` | New: boundary label builder helpers |
| `errors.go` | New: ErrNotBoundToBoundary sentinel |
| `boundary_test.go` | New: unit tests for boundary API |
| `examples/core/boundary-usage/main.go` | New: example usage |

### Testing Strategy

1. **Unit tests:**
   - Boundary define/bind validation
   - Key construction with boundary ID
   - Metadata encode/decode (SDK layer)
   - Index key maintenance (SADD/SREM)
   - BoundaryID data flow through SyncLockRequest → AcquireRequest
   - Define-time rejection of lineage + boundary

2. **Integration tests:**
   - Two usecases in same boundary → mutual exclusion (one acquires, other fails)
   - ActiveLocks() accuracy (returns correct usecase name, owner, timestamps)
   - ActiveLocks() self-heals orphaned index entries (simulate TTL expiry)
   - Force release behavior (cleans all keys, idempotent on non-existent lock)
   - Strict mode + boundary interaction (fencing tokens work correctly)
   - Hold usecase + boundary (metadata written correctly, visible via ActiveLocks)
   - Renew refreshes both lease and metadata TTL

3. **Backward compatibility tests:**
   - Non-boundary usecases unchanged (plain ownerID in lease key)
   - Mixed environment (some boundary, some not) coexist in same Redis instance

### Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| Additional Redis key per lock (metadata + index) | Benchmark memory impact; metadata key has same TTL as lease; index is bounded by active locks |
| Lua script complexity (3-key atomic ops) | Scripts remain simple; 3-key ops are well within Redis limits |
| Boundary name collision across teams | Document that boundary names should be globally unique within shared Redis; consider namespace prefix if needed |
| Index key drift (orphaned entries) | ActiveLocks() self-heals via SSCAN + EXISTS check + SREM cleanup |
| Metrics cardinality explosion | Document bounded boundary names; add config to disable per-usecase metrics |
| Lineage incompatibility | Enforced at define time; documented restriction; future enhancement tracked |
