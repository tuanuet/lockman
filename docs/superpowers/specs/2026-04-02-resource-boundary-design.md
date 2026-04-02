# Resource Boundary Design

## Problem Statement

Hiện tại, lock key được xây dựng từ `DefinitionID:resourceKey`, trong đó `DefinitionID` hash từ `useCaseName + kind`. Điều này có nghĩa là 2 usecase khác nhau (vd: `order.process` và `order.cancel`) không thể mutual exclude trên cùng 1 resource (vd: `order:123`) — chúng tạo ra 2 Redis key khác nhau.

**Yêu cầu:** Cho phép nhiều usecase chia sẻ cùng 1 lock boundary để:
1. Mutual exclusion trên cùng resource
2. Cross-usecase visibility (query "usecase nào đang giữ lock")
3. Explicit API — khai báo rõ ràng lúc define

## Solution: ResourceBoundary

### Core Concept

`ResourceBoundary` là 1 abstraction độc lập với usecase, đại diện cho 1 "lock domain". Nhiều usecase bind vào cùng boundary → shared lock key + cross-usecase visibility.

### API Design

#### 1. Define Boundary

```go
orderBoundary := lockman.DefineResourceBoundary("order")
```

- Boundary name phải unique trong 1 client instance
- Panic nếu trùng name: `"lockman: boundary 'order' already defined"`

#### 2. Bind Usecases vào Boundary

```go
processUC := lockman.DefineRun("order.process", orderBoundary.Bind())
cancelUC := lockman.DefineRun("order.cancel", orderBoundary.Bind())
```

- `orderBoundary.Bind()` trả về `UseCaseOption`
- Usecases bind vào boundary dùng boundary's ID thay vì definitionID cho lock key

#### 3. Query Active Locks

```go
activeLocks, err := orderBoundary.ActiveLocks(ctx)
// Returns: []BoundaryLockInfo{
//   {ResourceKey: "order:123", UsecaseName: "order.process", OwnerID: "...", AcquiredAt: ..., ExpiresAt: ...},
// }
```

#### 4. Force Release (Admin Operation)

```go
err := orderBoundary.Release(ctx, "order:123")
```

- Release bất kỳ lock nào trong boundary, không cần owner validation
- Dùng cho admin/ops scenarios (cleanup, recovery)

### Key Construction

#### Before (current)
```
lockman:lease:<definitionID>:<resourceKey>
```

#### After (với boundary)
```
lockman:lease:bdry_<boundaryID_hash>:<resourceKey>
```

- `boundaryID` = stable hash từ boundary name (dùng FNV-64a, format: `bdry_<hex>`)
- Non-boundary usecases không đổi — backward compatible

### Lease Metadata Format

#### Before (current)
```
Redis value = ownerID (plain string)
```

#### After (với boundary)
```json
{"o":"owner123","u":"order.process","a":"2026-04-02T10:00:00Z","e":"2026-04-02T10:00:30Z"}
```

Fields:
- `o` = ownerID
- `u` = usecaseName (người giữ lock)
- `a` = acquiredAt timestamp
- `e` = expiresAt timestamp (acquiredAt + leaseTTL)

**Backward compatibility:** Backend detect format lúc read — nếu không phải JSON → treat as legacy plain ownerID. Legacy locks vẫn hoạt động bình thường, chỉ không có visibility metadata.

### Integration với Existing Features

#### Lineage Mode

Boundary + lineage kết hợp khi parent boundary có child usecases:

```go
parentBoundary := lockman.DefineResourceBoundary("order")
childUC := lockman.DefineRun("order.fulfill", 
    parentBoundary.Bind(),
    lockman.WithLineage(parentBoundary),
)
```

- Lineage key format: `lockman:lease:lineage:bdry_<hash>:<resourceKey>`
- Child acquire check parent boundary lease → reject nếu parent đang hold
- Parent boundary visibility includes child lineage metadata

#### Strict Mode

Strict mode (fencing token) hoạt động bình thường với boundary:

- Fence counter key: `lockman:lease:fence:bdry_<hash>:<resourceKey>`
- Token key: `lockman:lease:strict-token:bdry_<hash>:<resourceKey>`
- Fencing token validation không đổi — chỉ ownerID check, không phụ thuộc usecase name

### Error Handling

#### Boundary Collision
```go
DefineResourceBoundary("order") // ok
DefineResourceBoundary("order") // panic: "lockman: boundary 'order' already defined"
```

#### Cross-Boundary Release
```go
// Release từ usecase không bind vào boundary → ErrNotBoundToBoundary
// Force release từ boundary object → luôn thành công (admin operation)
```

#### Stale Visibility Data
```go
type BoundaryLockInfo struct {
    ExpiresAt time.Time // Caller check IsZero() hoặc After(time.Now())
}
```
`ActiveLocks()` có thể return stale data (lock đã expire giữa lúc query) → metadata có `expires_at` để caller tự validate.

### Observability

Boundary expose metrics riêng:

```
# Prometheus-style metrics
lockman_boundary_locks_active{boundary="order"} → gauge
lockman_boundary_acquires_total{boundary="order", usecase="order.process"} → counter
lockman_boundary_contentions_total{boundary="order"} → counter
```

### Migration Path

#### Phase 1: Add Boundary API
- Implement `DefineResourceBoundary()`, `Bind()`, key builder changes
- Backward compatible — non-boundary usecases không đổi

#### Phase 2: Migrate Lease Metadata Format
- Change lease value từ plain string → JSON
- Detect legacy format lúc read (backward compat)

#### Phase 3: Visibility API + Metrics
- Implement `ActiveLocks()`, `Release()`
- Add Prometheus metrics

### File Changes (Dự kiến)

| File | Change |
|------|--------|
| `boundary.go` | New: Boundary type, DefineResourceBoundary, Bind |
| `backend/redis/driver.go` | Modify: buildLeaseKey support boundary ID, JSON metadata |
| `backend/redis/scripts.go` | Modify: acquire/release scripts handle JSON format |
| `internal/sdk/usecase.go` | Modify: support boundary ID resolution |
| `lockkit/runtime/exclusive.go` | Modify: resolve boundary ID trước khi acquire |
| `metrics.go` | New: boundary-specific metrics |
| `boundary_test.go` | New: unit tests for boundary API |
| `examples/core/boundary-usage/main.go` | New: example usage |

### Testing Strategy

1. **Unit tests:**
   - Boundary define/bind validation
   - Key construction với boundary ID
   - JSON metadata encode/decode
   - Legacy format detection

2. **Integration tests:**
   - 2 usecases cùng boundary → mutual exclusion
   - ActiveLocks() accuracy
   - Force release behavior
   - Lineage + boundary interaction
   - Strict mode + boundary interaction

3. **Migration tests:**
   - Legacy lock → boundary usecase compatibility
   - Mixed environment (some boundary, some not)

### Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| JSON metadata overhead (CPU/memory) | Benchmark impact, keep format minimal (short keys) |
| Breaking change cho existing deployments | Backward compat: detect legacy format, gradual migration |
| Boundary name collision trong distributed system | Boundary là client-side concept, không cần distributed coordination |
| Visibility query performance (scan keyspace) | Dùng pattern scan với limit, hoặc maintain index key |
