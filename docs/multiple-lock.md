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

All requests are built via `RunUseCase.With` — keys go through the definition's `KeyBuilder` for type safety.

```go
orderDef := lockman.DefineLock(
    "order",
    lockman.BindResourceID("order", func(in BatchInput) string { return in.OrderID }),
)
batchUC := lockman.DefineRunOn("batch_process", orderDef)

req1, _ := batchUC.With(BatchInput{OrderID: "1"})
req2, _ := batchUC.With(BatchInput{OrderID: "2"})
req3, _ := batchUC.With(BatchInput{OrderID: "3"})

err := client.RunMultiple(ctx, func(ctx context.Context, lease lockman.Lease) error {
    // lease.ResourceKeys = ["order:1", "order:2", "order:3"]
    return processBatch(ctx, lease.ResourceKeys)
}, []lockman.RunRequest{req1, req2, req3})
```

### Behavior

- **All-or-nothing**: if any key fails to acquire, all previously acquired keys are released
- **Canonical ordering**: keys are sorted alphabetically before acquisition (prevents deadlocks)
- **Overlap rejection**: if any key overlaps with existing locks, the entire operation is rejected
- **Max keys**: 100 keys per call
- **Same use case**: all requests must belong to the same use case

## HoldMultiple

Acquires multiple keys and returns a single `HoldHandle`. Keys remain locked until `Forfeit` is called.

All requests are built via `HoldUseCase.With` — keys go through the definition's `KeyBuilder` for type safety.

```go
slotDef := lockman.DefineLock(
    "slot",
    lockman.BindResourceID("slot", func(in ReserveInput) string { return in.SlotID }),
)
holdUC := lockman.DefineHoldOn("reserve_slots", slotDef)

req1, _ := holdUC.With(ReserveInput{SlotID: "A"})
req2, _ := holdUC.With(ReserveInput{SlotID: "B"})
req3, _ := holdUC.With(ReserveInput{SlotID: "C"})

handle, err := client.HoldMultiple(ctx, []lockman.HoldRequest{req1, req2, req3})
// ... manual workflow steps ...
client.Forfeit(ctx, holdUC.ForfeitWith(handle.Token()))
```

### Behavior

- Same all-or-nothing acquisition as `RunMultiple`
- Single `HoldHandle` manages all keys
- Renewal is handled by the hold manager for all keys
- `Forfeit` releases all keys at once
- **Same use case**: all requests must belong to the same use case

## Validation

| Condition | Error |
|---|---|
| Empty requests list | `requests must not be empty` |
| Duplicate keys | `duplicate key "..."` |
| More than 100 requests | `requests must not exceed 100` |
| Mixed use cases | `all requests must belong to the same use case` |
| Strict definition | `lockman: backend lacks required capability` |
| Use case not registered | `lockman: use case not found` |
| Registry mismatch | `lockman: use case does not belong to this registry` |

## Examples

- `examples/sdk/multiple-run/` — RunMultiple with Redis backend
- `examples/sdk/multiple-hold/` — HoldMultiple with Redis backend
