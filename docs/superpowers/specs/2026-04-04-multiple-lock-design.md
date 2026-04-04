# Multiple Lock (RunMultiple / HoldMultiple) Design

## Problem Statement

Currently, lockman supports acquiring multiple locks through **composite** use cases, but composite is designed for **different definitions** (e.g., inventory lock + payment lock). There is no first-class way to acquire **multiple keys of the same definition** in one atomic operation.

Real-world scenarios that need this:

1. **Batch order processing**: Process 5 orders at once, need exclusive access to all 5 order IDs before executing
2. **Multi-resource reservation**: Hold locks on several warehouse slots for a manual workflow, release together later
3. **Aggregate operations**: "Lock all orders for user X" — the set of keys is dynamic, not known at definition time

Workarounds today:

- Call `Run` N times in a loop — no atomicity, partial failure risk, manual rollback
- Define a composite with N members — impractical when key count is dynamic
- Use a single parent lock — loses per-key granularity, blocks unrelated operations on other keys

## Core Concept

### Multiple vs Composite

| | Composite | Multiple |
|---|---|---|
| Definitions | N different definitions | **1 definition** |
| Keys | 1 key per definition | **N keys, same definition** |
| Key count | Fixed at definition time | **Dynamic at call time** |
| Use case | Cross-resource coordination | **Batch same-type operations** |
| Error semantics | Per-member errors (some may succeed) | **All-or-nothing** (succeed all or fail all) |

### Design Principle

Multiple is a **runtime concern**, not a definition concern. The definition stays the same. What changes is how many keys of that definition are acquired together.

This means:

1. No new definition type needed
2. No registry changes needed
3. Backend contract unchanged — still `Acquire` per key
4. New API surface on `Client` only, plus internal engine support

## API Design

### RunMultiple

```go
func (c *Client) RunMultiple(
    ctx context.Context,
    uc RunUseCase[T],
    fn func(ctx context.Context, lease LeaseContext) error,
    input T,
    keys []string,
) error
```

Usage:

```go
orderDef := lockman.DefineLock(
    "order",
    lockman.BindKey(func(o Order) string { return o.ID }),
)
batchUC := lockman.DefineRunOn("batch_process", orderDef)

err := client.RunMultiple(ctx, batchUC, func(ctx context.Context, lease lockman.LeaseContext) error {
    // lease.ResourceKeys = ["order:1", "order:2", "order:3"]
    // All 3 keys are locked. Execute batch operation.
    return processBatch(ctx, lease.ResourceKeys)
}, input, []string{"order:1", "order:2", "order:3"})
```

### HoldMultiple

```go
func (c *Client) HoldMultiple(
    ctx context.Context,
    uc HoldUseCase[T],
    input T,
    keys []string,
) (HoldHandle, error)
```

The `input T` parameter is used for use case routing and observability (use case name, definition lookup). The `keys []string` parameter bypasses the binding function — keys are provided directly because the caller already knows which resources to lock. This is consistent with the design intent: multiple key sets are dynamic at call time, not derivable from a single input value.

Usage:

```go
slotDef := lockman.DefineLock(
    "warehouse_slot",
    lockman.BindKey(func(r SlotRequest) string { return r.SlotID }),
)
holdUC := lockman.DefineHoldOn("reserve_slots", slotDef)

handle, err := client.HoldMultiple(ctx, holdUC, input, []string{"slot:A", "slot:B", "slot:C"})
// All 3 slots locked. Handle manages renewal for all of them.
// ... manual workflow steps ...
client.Forfeit(ctx, handle) // Releases all 3 keys
```

### Forfeit

No API change needed, but the hold token construction must be specified. The existing `HoldHandle` encodes keys via `sdk.EncodeHoldToken([]string{keys...}, ownerID)`. For HoldMultiple, the token encodes **all keys** in the multiple acquire:

```go
token := sdk.EncodeHoldToken(keys, ownerID)
```

This matches the existing token format — the hold manager already decodes the token to extract keys on forfeit. No change to the token protocol is needed.

## Execution Semantics

### All-or-Nothing

1. Validate keys (non-empty, no duplicates)
2. Canonical sort keys → deterministic order (prevents deadlocks)
3. Acquire each key sequentially in sorted order
4. If **any** key fails to acquire:
   - Release all previously acquired keys (reverse order)
   - Return the acquire error (wrapped with context)
5. If **all** keys succeed:
   - Build `LeaseContext` with aggregated `ResourceKeys`
   - TTL = minimum of all member TTLs
   - Deadline = earliest member deadline
   - Call user callback
6. After callback:
   - Release all keys in reverse canonical order
   - Combine release errors if any

### Canonical Ordering

Same as composite: sort by (rank, resource name, resource key). Since multiple uses 1 definition, ordering is effectively by resource key only.

### Overlap Rejection

Same as composite: if any of the requested keys overlap with existing locks held by the same identity (parent-child lineage rules), the entire multiple acquire is rejected before any acquisition happens.

### HoldMultiple Behavior

Same acquisition flow as RunMultiple, but:

1. After successful acquire, register renewal watchers for **all** keys
2. Return a single `HoldHandle` that tracks all leases
3. Renewal failure for **any** key cancels the handle (all keys treated as lost)
4. `Forfeit` releases all keys tracked by the handle

## Validation Rules

1. Keys must be non-empty
2. Keys must have no duplicates
3. Definition must not be strict — returns `ErrBackendCapabilityRequired` at client method level (consistent with how `Run` rejects strict when backend doesn't support it)
4. Definition must not be composite (a composite definition cannot be used with Multiple)
5. Input must bind cleanly to each key via the definition's binding function
6. Key count must not exceed 100 — returns error if exceeded (prevents accidental large acquisitions)

## Internal Engine

### New File: `lockkit/runtime/multiple.go`

```go
func (m *Manager) ExecuteMultipleExclusive(
    ctx context.Context,
    req definitions.MultipleLockRequest,
    fn func(context.Context, definitions.LeaseContext) error,
) error
```

Flow:

1. Check shutdown
2. Call `tryAdmitInFlightExecution()` — count against in-flight limit (same as composite)
3. Defer `releaseInFlightExecution()`
4. Load definition from registry
5. Validate: not strict, not composite
6. Validate keys: non-empty, no duplicates
7. For each key:
   - Build acquire plan from definition + key
8. Canonicalize plan (sort by resource key)
9. Reject overlap (pre-acquisition check)
10. Apply runtime overrides
11. Install reentrancy guards
12. Acquire each key in canonical order
13. If any fails → release all acquired → return error
14. Build `LeaseContext{ResourceKeys, aggregated TTL/deadline}`
15. Call callback
16. Release all keys in reverse order

### New Request Type: `definitions.MultipleLockRequest`

```go
type MultipleLockRequest struct {
    DefinitionID string
    Keys         []string
    Ownership    OwnershipMeta
    Overrides    *RuntimeOverrides
}
```

### HoldMultiple Integration

HoldMultiple uses the same internal acquire loop as RunMultiple, but instead of calling a callback and releasing immediately, it:

1. Acquires all keys via `ExecuteMultipleExclusive` with a no-op callback (acquire-only mode)
2. Encodes all keys into a single hold token: `sdk.EncodeHoldToken(keys, ownerID)`
3. Registers each lease with the existing hold manager for renewal
4. Returns a `HoldHandle` containing the encoded token

The hold manager does not need structural changes — it already tracks leases by decoded token keys. Renewal, cancellation, and forfeit all work through the existing token decode → key extraction → per-key operation path.

## Backend Impact

**None.** Multiple is purely an orchestration concern. The backend still receives individual `Acquire`/`Release` calls. No new driver methods or capabilities needed.

## Error Handling

| Error | When |
|---|---|
| `ErrBusy` | At least one key is held by another identity |
| `ErrTimeout` | Timeout while waiting for any key |
| `ErrShuttingDown` | Client is shutting down |
| `ErrUseCaseNotFound` | Use case not registered |
| Wrapped acquire error | Partial failure during multi-key acquire (with context about which key failed) |

Release errors after partial failure are logged but do not override the original acquire error.

## Observability

Observability continues to use the use case name for user-facing events.

Events for RunMultiple use the same constants as single-key operations (`EventAcquireStarted`, `EventAcquireCompleted`, `EventAcquireFailed`, `EventCallbackStarted`, `EventCallbackCompleted`, `EventReleased`), but with per-key granularity matching the composite pattern:

1. `EventAcquireStarted` — emitted per key, with `resource_key` set to the specific key being acquired
2. `EventAcquireCompleted` — emitted per key, when that key is successfully acquired
3. `EventAcquireFailed` — emitted for the failed key with error context; remaining keys are not attempted
4. `EventCallbackStarted` / `EventCallbackCompleted` — single aggregated events with `resource_keys` (plural) listing all acquired keys
5. `EventReleased` — emitted per key, in reverse canonical order

The inspect store already handles `ResourceKeys` (plural) from composite operations. Multiple lock entries will use the same `resource_keys` field — no additional tracking changes needed.

## File Changes (Expected)

| File | Change |
|------|--------|
| `client_multiple.go` | New: `RunMultiple`, `HoldMultiple` public methods |
| `lockkit/definitions/ownership.go` | New: `MultipleLockRequest` type |
| `lockkit/runtime/multiple.go` | New: `ExecuteMultipleExclusive` |
| `lockkit/holds/manager.go` | No change needed — uses existing token decode + per-key renewal |
| `client_validation.go` | Add: reject strict/composite definitions for multiple |
| `observe/dispatcher.go` | Extend: event payloads support multiple keys |
| `inspect/store.go` | No change needed — already supports `ResourceKeys` from composite |
| `examples/sdk/multiple-run/` | New: example for RunMultiple |
| `examples/sdk/multiple-hold/` | New: example for HoldMultiple |
| `docs/multiple-lock.md` | New: documentation |
| `*_test.go` | New: unit + integration tests |

## Testing Strategy

### Unit Tests

1. `RunMultiple` acquires all keys and calls callback
2. `RunMultiple` fails fast if any key is busy (all-or-nothing)
3. `RunMultiple` releases acquired keys on partial failure
4. `RunMultiple` rejects empty keys list
5. `RunMultiple` rejects duplicate keys
6. `RunMultiple` rejects strict definitions
7. `RunMultiple` rejects composite definitions
8. `RunMultiple` canonical ordering is deterministic
9. `RunMultiple` TTL is minimum of all members
10. `HoldMultiple` returns handle tracking all keys
11. `HoldMultiple` renewal failure cancels all keys
12. `Forfeit` releases all keys in multi-lease handle
13. Overlap rejection: multiple acquire rejected if any key overlaps

### Integration Tests

1. `RunMultiple` with Redis backend — all keys acquired, callback executed
2. `RunMultiple` concurrent contention — two goroutines competing for overlapping key sets
3. `HoldMultiple` with Redis backend — hold, renew, forfeit lifecycle
4. `RunMultiple` + single `Run` on overlapping key — mutual exclusion works
5. `HoldMultiple` + `Run` on overlapping key — mutual exclusion works

### Backward Compatibility

1. Single-key `Run` / `Hold` behavior unchanged
2. Composite behavior unchanged
3. Registry validation unchanged (multiple doesn't create new definitions)

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Large key sets cause long acquisition times | Document recommended max key count; add validation for unreasonable sizes |
| Partial release failure leaves orphan locks | Release errors are logged; TTL ensures eventual cleanup |
| HoldMultiple renewal storm for many keys | Stagger renewal timing per key; same mechanism as composite |
| API surface confusion between composite and multiple | Clear docs and examples; different method names (`RunMultiple` vs `DefineCompositeRun`) |
| Callback receives keys but caller can't tell which key failed on error | Error wrapping includes the failed key; observability events include per-key detail |

## Recommendation

Implement `RunMultiple` and `HoldMultiple` as client-level methods backed by a new `ExecuteMultipleExclusive` in the runtime engine. No backend changes, no registry changes, no new definition types. This keeps the feature focused, incremental, and consistent with the existing composite pattern while serving a distinct use case (dynamic key sets on a single definition).
