# Shared Lock Definition Design

## Problem Statement

Currently, lock identity is derived directly from each use case name. In practice, this means:

1. Lock identity is coupled to execution surface
2. Two use cases that operate on the same resource cannot naturally share one lock identity
3. Reusing only a binding function is not enough to guarantee mutual exclusion

The desired model is definition-first:

1. Define a shared lock identity first
2. Let multiple use cases use that lock identity in different execution modes
3. Ensure the same definition and resource key are always mutually exclusive, regardless of whether the operation is run, hold, or claim
4. Let composite use cases acquire existing definitions atomically instead of inventing new lock identities
5. Keep existing shorthand APIs working for callers that do not need shared definitions

Lineage is out of scope for this design.

## Core Concept

### LockDefinition Is the Primitive

A `LockDefinition[T]` represents the shared lock identity for one resource domain.

It owns:

1. Public definition name
2. Stable internal `definitionID`
3. Typed `Binding[T]`
4. Definition-level lock semantics such as strictness

It does not encode execution mode. Execution mode belongs to a use case.

This means the following use cases can all share the same lock identity:

```go
contractDef := lockman.DefineLock(
    "contract",
    lockman.BindResourceID("order", func(o Order) string { return o.ID }),
    lockman.Strict(),
)

importUC := lockman.DefineRunOn("import", contractDef)
deleteUC := lockman.DefineRunOn("delete", contractDef)
holdUC := lockman.DefineHoldOn("manual_hold", contractDef)
claimUC := lockman.DefineClaimOn("process", contractDef, lockman.Idempotent())
```

All four use cases are mutually exclusive on the same bound resource key.

## Semantics

### Lock Identity

Lock exclusion is determined by:

```text
definitionID + resourceKey
```

Not by use case name.

If two operations resolve to the same `definitionID` and the same `resourceKey`, they must reject each other, even if they come from different use cases or different execution modes.

Examples:

1. `DefineRunOn("import", contractDef)` conflicts with `DefineRunOn("delete", contractDef)` on `order:123`
2. `DefineRunOn("import", contractDef)` conflicts with `DefineHoldOn("manual_hold", contractDef)` on `order:123`
3. `DefineClaimOn("process", contractDef)` conflicts with `DefineRunOn("import", contractDef)` on `order:123`

### Use Case Name

Use case name remains useful for:

1. Public registration
2. Logging and observability
3. Callback routing
4. Error reporting

But it does not define lock identity.

`definitionID` remains an internal identifier. Public APIs should continue to expose user-facing names rather than the shared internal definition identity.

## API Design

### 1. Define a Lock Definition

```go
contractDef := lockman.DefineLock(
    "contract",
    lockman.BindResourceID("order", func(o Order) string { return o.ID }),
    lockman.Strict(),
)
```

Proposed shape:

```go
type LockDefinition[T any] struct {
    name    string
    id      string
    binding Binding[T]
    config  definitionConfig
}

func DefineLock[T any](name string, binding Binding[T], opts ...DefinitionOption) LockDefinition[T]
```

Rules:

1. Definition name is required
2. Binding is required
3. `definitionID` is stable and derived from definition name only
4. The same definition can be used by many use cases
5. Definition options apply to every use case using that definition

### 2. Use a Definition in Single-Resource Use Cases

```go
importUC := lockman.DefineRunOn("import", contractDef)
deleteUC := lockman.DefineRunOn("delete", contractDef)
holdUC := lockman.DefineHoldOn("manual_hold", contractDef)
claimUC := lockman.DefineClaimOn("process", contractDef, lockman.Idempotent())
```

Proposed public constructors:

```go
func DefineRunOn[T any](name string, def LockDefinition[T], opts ...UseCaseOption) RunUseCase[T]
func DefineHoldOn[T any](name string, def LockDefinition[T], opts ...UseCaseOption) HoldUseCase[T]
func DefineClaimOn[T any](name string, def LockDefinition[T], opts ...UseCaseOption) ClaimUseCase[T]
```

Rules:

1. Use case name is unique in the registry
2. Many use cases may reference the same definition
3. Use case options remain execution-specific
4. Mutual exclusion is shared across run, hold, and claim when they reference the same definition

The existing shorthand constructors remain unchanged:

```go
func DefineRun[T any](name string, binding Binding[T], opts ...UseCaseOption) RunUseCase[T]
func DefineHold[T any](name string, binding Binding[T], opts ...UseCaseOption) HoldUseCase[T]
func DefineClaim[T any](name string, binding Binding[T], opts ...UseCaseOption) ClaimUseCase[T]
```

Those shorthand forms create an implicit private definition behind the scenes.

### 3. Composite Use Cases Use Existing Definitions

```go
syncUC := lockman.DefineCompositeRun("sync",
    lockman.Member("setting", settingDef, func(input SyncInput) SettingInput {
        return SettingInput{SettingID: input.SettingID}
    }),
    lockman.Member("contract", contractDef, func(input SyncInput) Order {
        return Order{ID: input.OrderID}
    }),
)
```

Proposed public shape:

```go
type CompositeMember[TInput any] struct {
    name  string
    build func(TInput) (definitionID string, resourceKey string, err error)
}

func Member[TInput any, TMember any](
    name string,
    def LockDefinition[TMember],
    project func(TInput) TMember,
) CompositeMember[TInput]

func DefineCompositeRun[T any](name string, members ...CompositeMember[T]) RunUseCase[T]
```

Rules:

1. Composite members reference definitions directly
2. Each member includes a projection from composite input to that definition's typed input
3. Composite member name is only a label for ordering and observability
4. Composite members do not create new lock identities
5. A composite acquire must conflict with standalone acquires of the same member definition and resource key

`Member(...)` is responsible for combining:

1. The referenced definition's internal identity
2. The definition's binding
3. The member projection function

This keeps the public API typed while allowing composite members to use a non-generic internal representation that is easier to implement in Go.

### 4. Force Release Belongs to the Definition

```go
err := contractDef.ForceRelease(ctx, client, "order:123")
```

Proposed shape:

```go
func (d LockDefinition[T]) ForceRelease(ctx context.Context, client *Client, resourceKey string) error
```

Force release is an operation on lock identity, not on use case name.

## Execution Options

Definition-level options:

1. `Strict()`

Use-case-level options:

1. `TTL(...)`
2. `WaitTimeout(...)`
3. `Idempotent()`

This split keeps shared lock semantics attached to the definition while still allowing execution behavior to vary per use case where the engine model can represent it.

Restrictions:

1. Hold use cases may not reference a strict definition
2. When a definition is shared by multiple use cases, non-zero `TTL(...)` values must agree across those use cases
3. When a definition is shared by multiple use cases, non-zero `WaitTimeout(...)` values must agree across those use cases

Example:

```go
contractDef := lockman.DefineLock(
    "contract",
    lockman.BindResourceID("order", func(o Order) string { return o.ID }),
    lockman.Strict(),
)

importUC := lockman.DefineRunOn("import", contractDef, lockman.TTL(30*time.Second))
deleteUC := lockman.DefineRunOn("delete", contractDef, lockman.WaitTimeout(5*time.Second))
```

Both share the same lock identity and strict lock semantics. Shared definitions may still vary by execution-specific options such as `Idempotent()`, but `TTL(...)` and `WaitTimeout(...)` must not conflict across attached use cases.

## Engine and Registry Implications

This design requires a change in internal modeling.

### Current Limitation

Today, the system effectively normalizes each registered use case into its own engine lock definition. That prevents multiple public use cases from sharing one `definitionID`.

### Required Model Change

Startup planning must separate:

1. Public use case registration
2. Unique lock definition registration

The planner should:

1. Register all public use cases by name for SDK routing
2. Collect unique `LockDefinition`s referenced by those use cases
3. Register engine definitions from those unique lock definitions, not from use case names
4. Normalize each execution surface separately so run, hold, and claim can all reference the same underlying definition

This is the core architectural change that enables shared lock identity.

### Composite Planning Requirement

Composite planning must operate on existing definitions.

That means the engine representation must support the equivalent of:

1. A composite operation references a set of existing definition IDs
2. Acquire is atomic across that set
3. Conflict detection remains consistent with standalone acquires of those same definitions
4. Each member carries its own projected resource key derived from the composite input

No fallback that invents composite-specific member definition IDs is acceptable for this feature. Composite members must reuse the same shared definition IDs as standalone acquires.

If the current lockkit composite model only supports member IDs derived from a composite-specific parent ID, that model must be extended for this feature.

### Shared Definition Across Execution Modes

One `LockDefinition` may be referenced by run, hold, and claim use cases at the same time.

The internal representation must therefore support:

1. One shared definition identity
2. Multiple execution surfaces referencing that identity
3. Mutual exclusion that is enforced consistently across sync and async paths

Normalization rule:

1. If a shared definition is referenced by both sync and async use cases, it must normalize to `ExecutionBoth`

If the current engine representation ties definition identity to one execution kind, that model must be extended before exposing the public API.

## Backward Compatibility

Existing code should continue to work.

### Shorthand API

The current shorthand can remain:

```go
approveUC := lockman.DefineRun(
    "approve",
    lockman.BindResourceID("order", func(id string) string { return id }),
)
```

This should behave as sugar for an implicit private definition:

```go
approveDef := lockman.DefineLock(
    "approve",
    lockman.BindResourceID("order", func(id string) string { return id }),
)
approveUC := lockman.DefineRunOn("approve", approveDef)
```

Implications:

1. Existing callers keep working unchanged
2. Existing single-use-case behavior remains unchanged
3. Explicit shared definitions are only needed when multiple use cases must share one lock identity
4. Explicit shared-definition APIs use `DefineRunOn`, `DefineHoldOn`, and `DefineClaimOn` to avoid Go overloading issues

## Validation Rules

The design should enforce the following:

1. Definition name is required
2. Definition binding is required
3. Duplicate definition names are rejected
4. Use case name is required
5. Duplicate public use case names are rejected
6. Composite member name is required
7. Composite member definitions must be valid
8. Composite member projection is required
9. Hold use cases cannot reference strict definitions
10. Hold use cases still reject unsupported options such as composite execution on the hold use case itself
11. Composite use cases still reject unsupported combinations such as strict mode if the existing engine cannot support them
12. Shared definitions reject conflicting non-zero `TTL(...)` values across attached use cases
13. Shared definitions reject conflicting non-zero `WaitTimeout(...)` values across attached use cases

No lineage validation is needed because lineage is not part of this design.

## Observability

Observability should continue to use public use case names for user-facing events.

This keeps traces and logs readable:

1. `import`
2. `delete`
3. `manual_hold`
4. `process`

Internally, those operations may resolve to the same `definitionID`.

## File Changes (Expected)

| File | Change |
|------|--------|
| `definition.go` | New: `LockDefinition[T]`, `DefineLock`, `ForceRelease`, stable definition ID helpers |
| `binding.go` | Keep binding logic; introduce definition-level options separate from use-case options |
| `usecase_run.go` | Support `DefineRunOn(name, def, opts...)` and shorthand compatibility |
| `usecase_hold.go` | Support `DefineHoldOn(name, def, opts...)` and shorthand compatibility |
| `usecase_claim.go` | Support `DefineClaimOn(name, def, opts...)` and shorthand compatibility |
| `registry.go` | Track use case metadata that references lock definitions |
| `client_validation.go` | Normalize unique lock definitions separately from use cases |
| `internal/sdk/usecase.go` | Accept explicit definition IDs during normalization |
| `backend/contracts.go` | Add optional force release capability if needed |
| `backend/redis/driver.go` | Implement force release by definition ID if supported |
| `definition_test.go` | New: definition-first API unit tests |
| `examples/core/...` | Update or add example showing shared definitions |

## Testing Strategy

### Unit Tests

1. `DefineLock` creates stable definition IDs
2. `DefineRunOn`, `DefineHoldOn`, and `DefineClaimOn` retain public use case names while sharing the same definition ID
3. Shorthand `DefineRun(name, binding, ...)` still creates an implicit private definition
4. Definition-level `Strict()` affects every use case using that definition
5. Composite members reference definitions and projections rather than copied bindings
6. `ForceRelease` uses the shared definition ID
7. Shared definitions used by both sync and async surfaces normalize to `ExecutionBoth`
8. Hold use cases reject strict definitions

### Integration Tests

1. Two run use cases using the same definition mutually exclude on the same resource
2. Run and hold using the same definition mutually exclude on the same resource
3. Claim and run using the same definition mutually exclude on the same resource
4. Composite acquire conflicts with standalone acquire of a shared member definition
5. Force release clears a shared definition lock and is idempotent

### Backward Compatibility Tests

1. Existing non-shared shorthand use cases behave the same as before
2. Mixed environments with implicit and explicit definitions coexist

## Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Internal planner still assumes one use case equals one definition | Make planner explicitly collect unique definitions first |
| Composite implementation cannot reference existing definitions | Extend composite engine model before exposing the public API |
| Shared definitions across sync and async are not representable today | Extend engine representation so multiple execution surfaces can share one definition identity |
| Shared definitions make observability ambiguous | Keep public use case name as the primary user-facing label |
| Explicit-definition APIs feel heavier than shorthand | Keep shorthand for private definitions and reserve `*On` APIs for shared definitions |

## Recommendation

Adopt a definition-first model and remove the variant concept entirely.

The right abstraction is:

1. `LockDefinition` defines lock identity
2. `DefineRunOn`, `DefineHoldOn`, and `DefineClaimOn` define execution semantics over that identity
3. Composite acquires existing definitions atomically

This matches the desired mental model, supports true shared mutual exclusion, and removes the need for lineage in this feature.
