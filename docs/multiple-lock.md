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
