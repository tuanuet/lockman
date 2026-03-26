# Lock Definition Reference

This document explains the public definition shapes in `lockkit/definitions`.

It is a usage reference for application teams registering locks in the central registry. Phase-specific design rationale still lives in the spec documents under `docs/superpowers/specs/`.

## `LockDefinition`

`LockDefinition` describes one logical lock that can later be used by `runtime`, `workers`, or both.

```go
type LockDefinition struct {
    ID                   string
    Kind                 LockKind
    Resource             string
    Mode                 LockMode
    ExecutionKind        ExecutionKind
    LeaseTTL             time.Duration
    WaitTimeout          time.Duration
    RetryPolicy          RetryPolicy
    BackendFailurePolicy BackendFailurePolicy
    FencingRequired      bool
    IdempotencyRequired  bool
    CheckOnlyAllowed     bool
    Rank                 int
    ParentRef            string
    OverlapPolicy        OverlapPolicy
    KeyBuilder           KeyBuilder
    Tags                 map[string]string
}
```

### Field reference

| Field | Type | Meaning | Typical guidance |
|---|---|---|---|
| `ID` | `string` | Stable registry identifier for the definition. Application code refers to this ID, not raw lock keys. | Keep it globally unique and business-readable, for example `OrderLock` or `InventoryReservation`. |
| `Kind` | `LockKind` | Whether this is a `parent` or `child` lock. | Use `parent` for aggregate-level invariants. Use `child` only for sub-resources that can be coordinated independently. |
| `Resource` | `string` | Logical resource family protected by the lock. | Keep this stable and coarse, for example `order`, `account`, `inventory_item`. |
| `Mode` | `LockMode` | Coordination strictness. | Phase 2 supports `standard` runtime behavior. `strict` remains part of the public model but not all strict semantics are implemented yet. |
| `ExecutionKind` | `ExecutionKind` | Which execution path can use the definition: `sync`, `async`, or `both`. | Use `sync` for `runtime`, `async` for `workers`, `both` only when one definition is intentionally shared. |
| `LeaseTTL` | `time.Duration` | Target lease duration for one acquire. Renewal loops use this as the renewal basis. | Set long enough to cover expected handler time plus jitter, but not so long that stale ownership lingers after crashes. |
| `WaitTimeout` | `time.Duration` | Maximum time an acquire attempt waits before timing out. | Use `0` for immediate behavior. Use a short bounded duration when contention is acceptable. |
| `RetryPolicy` | `RetryPolicy` | Registry-level retry hint for acquire behavior. | Present in the public shape for future policy evolution. Current Phase 2 execution paths do not implement internal retry loops from this field. |
| `BackendFailurePolicy` | `BackendFailurePolicy` | How callers should treat backend-side failures. | In practice, `fail_closed` is the safe default. Strict definitions must validate as fail-closed. |
| `FencingRequired` | `bool` | Whether persistence-side fencing is required. | Only meaningful for strict-mode designs. Registry validation requires it for strict definitions. |
| `IdempotencyRequired` | `bool` | Whether async execution requires an idempotency store and idempotency key. | Set this for queue/message flows where duplicate delivery must be absorbed safely. |
| `CheckOnlyAllowed` | `bool` | Whether advisory presence checks are allowed for this definition. | Enable only when you intentionally support `CheckPresence` as an operational/UI hint. |
| `Rank` | `int` | Ordering rank used by policy and composite canonicalization. | Lower rank acquires earlier. Keep rank stable so ordering remains deterministic. |
| `ParentRef` | `string` | Parent definition ID for child locks. | Required for `KindChild`; empty for parent locks. |
| `OverlapPolicy` | `OverlapPolicy` | Parent/child overlap behavior. | In Phase 2, child overlap is reject-first. Validation only accepts `reject`. |
| `KeyBuilder` | `KeyBuilder` | Deterministic builder from structured input to concrete lock key. | Always required. Prefer template builders so required fields stay explicit. |
| `Tags` | `map[string]string` | Immutable metadata attached to the definition. | Use for governance, reporting, and grouping such as domain, owner team, or criticality. |

### Field interactions

- `KindChild` implies `ParentRef` must point to a registered parent definition.
- `ModeStrict` implies stricter validation, including `FencingRequired`.
- `ExecutionKind=async` or `both` should be paired with `IdempotencyRequired` when duplicate delivery is unsafe.
- `OverlapPolicy` matters only for child definitions.
- `LeaseTTL` drives both lease renewal cadence and worker idempotency TTL derivation.
- `KeyBuilder` is part of correctness, not convenience. If key construction is unstable, the definition is unsafe.

### Current Phase 2 constraints

- Child overlap is reject-first; escalation is not implemented as runtime behavior.
- Standard composite execution is supported; strict composite execution is out of scope.
- Worker execution expects the definition to be `async` or `both`.
- Runtime sync execution expects the definition to be `sync` or `both`.

## `CompositeDefinition`

`CompositeDefinition` declares one approved multi-resource acquire plan.

```go
type CompositeDefinition struct {
    ID               string
    Members          []string
    OrderingPolicy   OrderingPolicy
    AcquirePolicy    AcquirePolicy
    EscalationPolicy EscalationPolicy
    ModeResolution   ModeResolution
    MaxMemberCount   int
    ExecutionKind    ExecutionKind
}
```

### Field reference

| Field | Type | Meaning | Typical guidance |
|---|---|---|---|
| `ID` | `string` | Stable registry identifier for the composite plan. | Name the business operation, for example `TransferComposite`. |
| `Members` | `[]string` | Ordered list of member `LockDefinition` IDs. | This is the declared membership list, not the final acquire order. |
| `OrderingPolicy` | `OrderingPolicy` | How runtime derives canonical acquire order. | Phase 2 supports only `canonical`. |
| `AcquirePolicy` | `AcquirePolicy` | Whether partial success is allowed. | Phase 2 supports only `all_or_nothing`. |
| `EscalationPolicy` | `EscalationPolicy` | How overlap/escalation should behave. | Phase 2 supports only reject semantics. |
| `ModeResolution` | `ModeResolution` | How member modes are reconciled. | Phase 2 supports only `homogeneous`. Members must resolve cleanly together. |
| `MaxMemberCount` | `int` | Upper bound for member count. | Keep this small. Large composites are usually a design smell. |
| `ExecutionKind` | `ExecutionKind` | Whether the composite is used in sync, async, or both paths. | Match it to the intended caller path just like `LockDefinition`. |

### Composite behavior notes

- Member resource keys come from the member definitions' `KeyBuilder`s.
- Runtime canonicalizes acquire order; callers should not try to nest lock calls manually.
- Composite worker execution derives idempotency retention conservatively from the longest member lease basis.
- Composite lease context reports all resource keys, while lease TTL visibility is bounded by the shortest currently held member lease.

## Supporting enums

### `LockKind`

- `parent`: aggregate/root lock
- `child`: sub-resource lock linked to `ParentRef`

### `LockMode`

- `standard`: pragmatic coordination
- `strict`: future-facing stricter coordination model

### `ExecutionKind`

- `sync`: runtime-only
- `async`: worker-only
- `both`: shared between runtime and workers

### `OverlapPolicy`

- `reject`: reject parent/child overlap
- `escalate`: reserved for future evolution; not supported as Phase 2 behavior

## Example

```go
definitions.LockDefinition{
    ID:                  "OrderClaim",
    Kind:                definitions.KindParent,
    Resource:            "order",
    Mode:                definitions.ModeStandard,
    ExecutionKind:       definitions.ExecutionAsync,
    LeaseTTL:            30 * time.Second,
    WaitTimeout:         0,
    BackendFailurePolicy: definitions.BackendFailClosed,
    IdempotencyRequired: true,
    CheckOnlyAllowed:    true,
    Rank:                10,
    KeyBuilder:          definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
    Tags: map[string]string{
        "domain": "orders",
        "owner":  "payments-platform",
    },
}
```
