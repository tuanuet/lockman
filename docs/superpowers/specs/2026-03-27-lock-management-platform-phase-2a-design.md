# Lock Management Platform SDK for Go: Phase 2a Design

## Status

Draft

## Relationship to Existing Specs

This document extends the decisions in:

- [2026-03-26-lock-management-platform-design.md](/Users/mrt/workspaces/boilerplate/lockman/docs/superpowers/specs/2026-03-26-lock-management-platform-design.md)
- [2026-03-26-lock-management-platform-phase-2-design.md](/Users/mrt/workspaces/boilerplate/lockman/docs/superpowers/specs/2026-03-26-lock-management-platform-phase-2-design.md)

The base spec defines the lock taxonomy and governance model. The Phase 2 design adds child locks, composite support, and reject-first overlap semantics.

Phase 2a closes a semantic gap left by Phase 2:

- `parent` and `child` relationships exist in the registry
- overlap policy exists in the public model
- but single-lock execution paths do not yet enforce distributed parent-child overlap unless the operation goes through a declared composite plan

Phase 2a makes parent-child overlap enforcement real for single-lock execution.

## Problem Statement

The current platform model says parent and child locks are meaningful coordination concepts, not just labels.

However, in the current Phase 2 implementation:

- `ExecuteCompositeExclusive` and `ExecuteCompositeClaimed` apply overlap rejection
- standalone `ExecuteExclusive` and `ExecuteClaimed` do not yet enforce distributed parent-child overlap

This creates a mismatch between declared registry semantics and observable runtime behavior:

- a parent definition and child definition can still acquire independently on separate processes
- `ParentRef` behaves more like metadata than an always-enforced rule
- teams may incorrectly assume parent-child overlap is globally enforced when it is only clearly enforced through composite plans

Phase 2a must remove that ambiguity.

## Goal

Enforce reject-first parent-child overlap for single-lock execution in both:

- `runtime.ExecuteExclusive`
- `workers.ExecuteClaimed`

The enforcement must:

- work across processes, not only in-process
- apply only to `standard` mode in Phase 2a
- preserve the Phase 2 reject-first policy
- derive lineage from registry-declared `ParentRef`
- require deterministic hierarchical key structure validated at registry time

## Non-Goals

- No support for strict-mode parent-child enforcement
- No support for escalation from child to parent
- No support for generic lineage inference from arbitrary custom key builders
- No attempt to infer hierarchy from raw strings at execution time
- No replacement of composite plans for true multi-lock operations

Composite definitions remain the only approved way to execute a multi-lock flow.

## Core Phase 2a Decisions

### Parent-Child Semantics Must Be Enforced Outside Composite Paths

After Phase 2a, `parent` and `child` are no longer just classification metadata plus composite-only policy.

They become execution semantics for standalone lock acquisition as well.

This means:

- acquiring a child lock must reject if the corresponding parent instance is currently held
- acquiring a parent lock must reject if any descendant instance under that same parent instance is currently held

### Standard Mode Only

Phase 2a applies only to `ModeStandard`.

Strict mode remains outside implementation scope because strict parent-child overlap raises additional questions about guarded writes, fencing, and transfer semantics that Phase 2a does not solve.

### Reject-First Only

Phase 2a inherits the Phase 2 policy:

- overlap behavior is reject-first
- no wait semantics are introduced for parent-child overlap
- no implicit escalation is supported

This keeps single-lock behavior aligned with composite overlap behavior.

### Hierarchical Template Builders Are Mandatory for Lineage Chains

Phase 2a requires child lineage to be provable from the registry, not guessed at execution time.

For any definition participating in a `ParentRef` chain:

- the definition must use a template-backed hierarchical key builder
- the child key template must preserve the parent key as a prefix
- the child required fields must be a strict superset or equal superset of the parent required fields
- validation must recurse all the way up the ancestor chain

Examples:

Valid:

- parent: `order:{order_id}`
- child: `order:{order_id}:item:{item_id}`
- grandchild: `order:{order_id}:item:{item_id}:allocation:{allocation_id}`

Invalid:

- parent: `order:{order_id}`
- child: `item:{item_id}`
- child: `order-item:{item_id}`
- child: `order:{another_id}:item:{item_id}`

## Why Composite Still Exists

Phase 2a does not reduce the role of `CompositeDefinition`.

The distinction remains:

- `parent` and `child` describe what kind of lock a definition is
- `composite` describes an approved plan for acquiring multiple locks together

After Phase 2a:

- single-lock parent-child conflicts are enforced directly
- multi-lock flows must still use composite plans

This keeps lock taxonomy and execution planning separate.

Composite execution must also remain lineage-aware when any member participates in a parent-child chain.

That means:

- `ExecuteCompositeExclusive` must route lineage-aware members through `LineageDriver`
- `ExecuteCompositeClaimed` must route lineage-aware members through `LineageDriver`
- non-lineage members in the same composite may continue to use the plain exact-key path

Without this rule, composite execution would become a bypass around Phase 2a enforcement.

## Required Contract Changes

### `KeyBuilder` Contract Narrowing

Phase 2a should narrow which key builders are allowed for lineage-aware definitions.

The current generic contract:

```go
type KeyBuilder interface {
    RequiredFields() []string
    Build(input map[string]string) (string, error)
}
```

is not sufficient to prove hierarchical ancestry for arbitrary implementations.

Phase 2a therefore introduces a practical restriction:

- definitions with no `ParentRef` may continue to use the existing builder contract
- definitions in a parent-child chain must use the SDK's template-backed builder type

That restriction requires explicit introspection support from `definitions`.

Phase 2a should therefore add an exported template-lineage view for the SDK's template-backed builder, for example through one of these patterns:

- an exported interface implemented by template-backed builders
- an exported helper in `definitions` that unwraps template metadata safely

At minimum, registry validation must be able to read:

- the raw template string
- the ordered placeholder field list

Without that API, the validator cannot prove that a child template preserves the parent template prefix.

This is intentionally strict.

## Registry Validation Changes

Registry validation must now enforce the following for every child definition:

1. `ParentRef` must refer to a registered parent definition
2. the child and parent must both be `ModeStandard`
3. the child must use a template-backed hierarchical builder
4. the parent must use a template-backed hierarchical builder
5. the child template must embed the full built parent template as its prefix
6. the child required fields must include all parent required fields
7. the parent chain must recurse without breaks, ambiguity, or cycles
8. `OverlapPolicy` must remain `reject`

For recursive chains:

- each level must preserve the lineage of its immediate parent
- by transitivity, the full ancestor chain becomes derivable
- recursive validation must keep a visited-set and reject circular `ParentRef` chains

Validation failure should happen at registry startup, not at first execution.

## New Internal Concept: Lineage

Phase 2a introduces the internal notion of a lineage path.

For a resolved definition plus request input, runtime must be able to derive:

- the concrete key for the definition itself
- the concrete key for every ancestor in the `ParentRef` chain

Example:

- child request input:
  - `order_id=123`
  - `item_id=line-1`
- child key:
  - `order:123:item:line-1`
- derived ancestor keys:
  - `order:123`

For deeper chains:

- grandchild key:
  - `order:123:item:line-1:allocation:alloc-9`
- ancestor keys:
  - `order:123:item:line-1`
  - `order:123`

This lineage is the basis for distributed overlap enforcement.

## Distributed Enforcement Model

Exact-key presence checks are not enough to implement full parent-child rejection across processes.

They are sufficient for:

- child acquire checking whether the exact parent instance key is held

They are not sufficient for:

- parent acquire checking whether any descendant under that parent instance is held

To solve that, Phase 2a introduces a distributed descendant index.

### Descendant Index

When a child or deeper descendant lease is acquired, the backend must also publish lineage membership entries keyed by every ancestor instance.

Conceptually:

- acquire child `order:123:item:line-1`
- publish a descendant membership entry under ancestor `order:123`

For deeper lineage:

- acquire `order:123:item:line-1:allocation:alloc-9`
- publish markers under:
  - `order:123:item:line-1`
  - `order:123`

When the descendant lease is released or expires, its lineage membership entries must also disappear.

### Marker Semantics

The descendant index must answer:

- "Does this ancestor instance currently have any active descendant lease?"

That is enough for parent-side rejection.

Phase 2a does not require full subtree enumeration for API consumers. It only requires an internal overlap signal.

However, the internal storage model must be multiplicity-safe.

That means:

- multiple descendants under the same ancestor instance may coexist
- releasing one descendant must not clear the ancestor state for other still-held descendants

So the backend representation must not be a single boolean marker.

It must instead use one of these equivalent models:

- one membership entry per active descendant lease
- a reference count keyed by ancestor instance plus unique lease identity

The Redis-first implementation should prefer per-lease membership because it is easier to reason about during expiry and cleanup.

Each membership entry must be uniquely attributable to one held descendant lease.

## Driver and Backend Impact

The distributed descendant index is coordination policy support, but it cannot be implemented purely above the driver with the current exact-key presence contract.

Phase 2a therefore requires extending the backend support model.

For the Redis-first implementation, the backend should maintain:

- the main lease key for the held lock
- lineage membership structures for each ancestor instance

The Redis implementation must make acquire, renew, and release semantics consistent for both:

- the main lease
- descendant membership entries

This is not just a consistency preference. It is a correctness requirement.

Phase 2a must not use a non-atomic "check overlap, then acquire, then publish lineage membership" sequence.

That sequence leaves a race window where:

- a parent acquire and a child acquire can both pass their pre-checks
- both can then acquire independent exact lease keys
- overlap rejection is violated across processes

So the backend contract must support one atomic decision boundary for lineage-aware acquire.

For Redis, the implementation should use a single atomic acquire script or equivalent backend primitive that:

1. checks ancestor exact lease presence when acquiring a child
2. checks descendant membership presence when acquiring a parent
3. acquires the main lease if no overlap exists
4. publishes descendant membership entries in the same atomic step when needed

Renew must also be lineage-aware.

When a descendant lease is successfully renewed:

- the main lease TTL must be extended
- all descendant membership entries associated with that lease identity must also be extended in the same atomic backend step

Otherwise a long-running child handler could retain its main lease while its ancestor membership expires, allowing a parent acquire to slip through incorrectly.

Release must likewise remove descendant membership entries in the same ownership-checked release path as the main lease, or in an equivalently safe atomic backend step.

The exact internal structure can be Redis-specific, but the higher-level contract should remain backend-agnostic where possible.

### SDK Contract Shape

Phase 2a should not overload the existing `drivers.Driver` contract with lineage-specific semantics for all backends immediately.

Instead, it should introduce a second backend capability dedicated to lineage-aware standard-mode execution.

Recommended shape:

```go
type LineageDriver interface {
    AcquireWithLineage(ctx context.Context, req LineageAcquireRequest) (LeaseRecord, error)
    RenewWithLineage(ctx context.Context, lease LeaseRecord, lineage LineageLeaseMeta) (LeaseRecord, LineageLeaseMeta, error)
    ReleaseWithLineage(ctx context.Context, lease LeaseRecord, lineage LineageLeaseMeta) error
}
```

Recommended companion shapes:

```go
type LineageAcquireRequest struct {
    AcquireRequest
    Kind         definitions.LockKind
    AncestorKeys []AncestorKey
    LeaseID      string
}

type AncestorKey struct {
    DefinitionID string
    ResourceKey  string
}

type LineageLeaseMeta struct {
    LeaseID      string
    Kind         definitions.LockKind
    AncestorKeys []AncestorKey
}
```

- `LeaseID` is a unique held-lease identity used to attribute descendant membership safely
- `AncestorKeys` contains the fully resolved concrete ancestor chain derived from `ParentRef`
- parent definitions typically have an empty `AncestorKeys` slice on acquire, but may still use descendant membership checks before acquire
- child and deeper descendant definitions carry the ancestor chain needed for acquire, renew, and release
- the exact resource key for the lock itself continues to come from `AcquireRequest.ResourceKeys[0]`

Where:

- `drivers.Driver` remains the generic exact-key contract from Phase 2
- `LineageDriver` is an additional optional capability
- `runtime` and `workers` use `LineageDriver` only when executing a definition that participates in a validated parent-child chain
- manager construction must fail fast if the registry contains lineage-aware definitions but the configured backend does not implement `LineageDriver`

This choice keeps Phase 2 compatibility intact while making Phase 2a implementation concrete.

For Redis:

- the Redis driver should implement both `drivers.Driver` and `LineageDriver`

For the in-memory `testkit` driver:

- it should also implement `LineageDriver` so unit tests can cover the same semantics without Redis

## Runtime Execution Changes

`runtime.ExecuteExclusive` must gain lineage-aware overlap enforcement as part of acquire, not as a separate best-effort pre-check.

### Parent Acquire

When acquiring a parent definition:

1. build the parent key from request input
2. resolve the ancestor-instance descendant membership key for that parent instance
3. atomically reject if any descendant membership exists under that parent instance
4. otherwise atomically acquire the main parent lease

### Child Acquire

When acquiring a child definition:

1. build the child key from request input
2. derive the concrete ancestor key chain from `ParentRef`
3. atomically reject if any ancestor exact lease is currently held
4. atomically acquire the main child lease
5. atomically publish descendant membership entries for all ancestors as part of the same acquire decision

### Release

On release of a child or deeper descendant:

- release the main lease
- remove descendant membership entries for every ancestor in the chain

This cleanup must be multiplicity-safe and ownership-safe.

Releasing one descendant lease must remove only the membership entries associated with that lease identity.

## Composite Execution Changes

Phase 2a must also update composite execution paths.

For both:

- `runtime.ExecuteCompositeExclusive`
- `workers.ExecuteCompositeClaimed`

member-by-member acquire must use lineage-aware backend operations whenever the member definition participates in a validated parent-child chain.

Required behavior:

- parent members in a chain must check descendant membership before acquire
- child members in a chain must check ancestor exact leases and publish descendant membership on acquire
- renew and release for lineage-aware composite members must also go through `LineageDriver`
- non-lineage members may continue to use the plain driver path

This keeps composite execution aligned with the new single-lock rules and prevents composite plans from bypassing distributed overlap enforcement.

## Worker Execution Changes

`workers.ExecuteClaimed` must apply the same lineage-aware atomic acquire semantics as `runtime.ExecuteExclusive`.

The worker lifecycle remains:

1. validate request
2. pre-acquire idempotency handling
3. attempt lineage-aware atomic acquire
4. run callback
5. stop renewal
6. persist terminal idempotency state
7. release lease and lineage membership entries

Important:

- idempotency remains message-scoped
- overlap enforcement remains resource-scoped
- `ErrLockBusy` and runtime parent-child overlap rejection must remain distinguishable
- successful lease renewal for lineage-aware child executions must also renew lineage membership TTL atomically

Parent-child runtime overlap should continue to be distinct from ordinary lease contention.

## Error Semantics

Phase 2a should preserve a distinct runtime-overlap path for parent-child conflict.

Recommended behavior:

- registry-time structural invalidity continues to use `ErrPolicyViolation`
- runtime parent-child overlap detected before acquire returns a dedicated error such as `ErrOverlapRejected`
- ordinary lease contention still returns `ErrLockBusy`

This distinction matters because:

- registry invalidity is a definition-policy conflict
- runtime overlap is a transient lineage conflict between live leases
- contention is ordinary lock competition on the same exact key

Worker outcome mapping should distinguish runtime overlap from registry invalidity.

With the current Phase 2 outcome mapping, this means:

- registry invalidity still maps to `drop`
- runtime parent-child overlap should map to `retry`

This is intentional for Phase 2a because runtime overlap is transient and may clear as soon as the blocking parent or child lease is released.

## Presence API Impact

Phase 2a does not change `CheckPresence` semantics.

Presence checks continue to query exact-key state only.

Phase 2a does not expose descendant-active state through `CheckPresence`.

Lineage membership remains an internal coordination mechanism used by execution paths, not a new public presence API surface.

## Observability

Phase 2a should add explicit overlap observability.

Useful signals:

- overlap rejection count by definition
- overlap rejection count by parent definition
- descendant marker publish/remove failures
- descendant membership renew failures
- lineage depth

These signals help distinguish:

- exact-key contention
- parent-child policy rejection
- backend drift or marker cleanup problems

## Migration Impact

Phase 2a introduces a stricter registry contract for child definitions.

Existing child definitions may fail validation if:

- they do not use hierarchical template builders
- their child key does not preserve the parent key prefix
- their field set does not contain all parent fields

This is intentional.

It is better to fail fast during migration than to preserve ambiguous lineage that cannot be enforced correctly in distributed execution.

Phase 2a also changes runtime behavior for applications that currently rely on nested parent-then-child or child-then-parent acquisition patterns.

After Phase 2a:

- nested in-process parent-child execution that previously succeeded may now be rejected
- cross-process parent-child execution that previously succeeded may now be rejected

Teams should audit application code for:

- nested parent-then-child acquire sequences
- nested child-then-parent acquire sequences
- flows that implicitly assumed `ParentRef` was metadata only

Those flows should be redesigned either as:

- a single parent-only operation
- a single child-only operation
- or an explicit composite plan when true multi-lock execution is intended

## Testing Requirements

Phase 2a test coverage should include:

- registry rejection for invalid child template shapes
- recursive lineage validation across more than one ancestor level
- runtime parent acquire rejected by active child on another process/backend client
- runtime child acquire rejected by active parent on another process/backend client
- worker parent acquire rejected by active child on another process/backend client
- worker child acquire rejected by active parent on another process/backend client
- composite parent member rejected by active child on another process/backend client
- composite child member rejected by active parent on another process/backend client
- descendant marker cleanup on normal release
- descendant marker cleanup on lease expiry
- descendant marker cleanup on renewal failure paths
- descendant membership TTL extension on successful renew
- manager startup failure when lineage-aware definitions are registered against a backend that lacks `LineageDriver`

## Rollout Strategy

Phase 2a should be delivered as a design and implementation increment after Phase 2, not folded back into Phase 2 retroactively.

This keeps the platform history clear:

- Phase 2: child/composite support and reject-first policy model
- Phase 2a: single-lock parent-child enforcement and lineage-backed distributed behavior

## Success Criteria

Phase 2a is complete when:

- child definitions validate only if their key builders preserve ancestor lineage
- `runtime.ExecuteExclusive` rejects distributed parent-child overlap on single-lock paths
- `workers.ExecuteClaimed` rejects distributed parent-child overlap on single-lock paths
- `ExecuteCompositeExclusive` and `ExecuteCompositeClaimed` route lineage-aware members through `LineageDriver`
- overlap behavior is reject-first and consistent with composite behavior
- multi-lock operations still require composite plans

## Open Questions Deferred

- whether strict mode should adopt the same lineage model later
- whether a more specific overlap error taxonomy beyond `ErrOverlapRejected` should be introduced
- whether presence APIs should eventually surface descendant-active state to application callers
