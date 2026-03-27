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

## Required Contract Changes

## `KeyBuilder` Contract Narrowing

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

The registry validator should inspect the concrete template metadata rather than treating all `KeyBuilder` implementations as equally valid for lineage-aware use.

This is intentionally strict.

## Registry Validation Changes

Registry validation must now enforce the following for every child definition:

1. `ParentRef` must refer to a registered parent definition
2. the child and parent must both be `ModeStandard`
3. the child must use a template-backed hierarchical builder
4. the parent must use a template-backed hierarchical builder
5. the child template must embed the full built parent template as its prefix
6. the child required fields must include all parent required fields
7. the parent chain must recurse without breaks or ambiguity
8. `OverlapPolicy` must remain `reject`

For recursive chains:

- each level must preserve the lineage of its immediate parent
- by transitivity, the full ancestor chain becomes derivable

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

When a child or deeper descendant lease is acquired, the backend must also publish lineage markers keyed by every ancestor instance.

Conceptually:

- acquire child `order:123:item:line-1`
- publish a descendant marker under ancestor `order:123`

For deeper lineage:

- acquire `order:123:item:line-1:allocation:alloc-9`
- publish markers under:
  - `order:123:item:line-1`
  - `order:123`

When the descendant lease is released or expires, its lineage markers must also disappear.

### Marker Semantics

The descendant index must answer:

- "Does this ancestor instance currently have any active descendant lease?"

That is enough for parent-side rejection.

Phase 2a does not require full subtree enumeration for API consumers. It only requires an internal yes/no overlap signal.

## Driver and Backend Impact

The distributed descendant index is coordination policy support, but it cannot be implemented purely above the driver with the current exact-key presence contract.

Phase 2a therefore requires extending the backend support model.

For the Redis-first implementation, the backend should maintain:

- the main lease key for the held lock
- lineage marker keys or membership structures for each ancestor instance

The Redis implementation must make acquire and release semantics consistent for both:

- the main lease
- descendant markers

This should be done atomically enough that marker presence does not drift materially from lease state.

The exact internal structure can be Redis-specific, but the higher-level contract should remain backend-agnostic where possible.

## Runtime Execution Changes

`runtime.ExecuteExclusive` must gain overlap enforcement before calling `Acquire`.

### Parent Acquire

When acquiring a parent definition:

1. build the parent key from request input
2. check whether the exact parent key is already held, as normal acquire already does
3. check whether the descendant index says any child or lower descendant is currently held under that parent instance
4. if yes, return overlap rejection immediately
5. otherwise continue with acquire

### Child Acquire

When acquiring a child definition:

1. build the child key from request input
2. derive the concrete ancestor key chain from `ParentRef`
3. check whether any ancestor exact key is currently held
4. if yes, return overlap rejection immediately
5. otherwise continue with acquire
6. if acquire succeeds, publish descendant markers for all ancestors

### Release

On release of a child or deeper descendant:

- release the main lease
- remove descendant markers for every ancestor in the chain

## Worker Execution Changes

`workers.ExecuteClaimed` must apply the same overlap enforcement as `runtime.ExecuteExclusive` before lock acquire.

The worker lifecycle remains:

1. validate request
2. pre-acquire idempotency handling
3. resolve overlap state
4. attempt acquire
5. run callback
6. stop renewal
7. persist terminal idempotency state
8. release lease and lineage markers

Important:

- idempotency remains message-scoped
- overlap enforcement remains resource-scoped
- `ErrLockBusy` and parent-child overlap rejection must remain distinguishable

Parent-child overlap should continue to map to policy violation style rejection rather than ordinary lease contention.

## Error Semantics

Phase 2a should preserve a distinct "policy rejection" path for parent-child overlap.

Recommended behavior:

- overlap detected before acquire returns `ErrPolicyViolation` or a more specific future overlap error
- ordinary lease contention still returns `ErrLockBusy`

This distinction matters because:

- overlap is a definition-policy conflict
- contention is ordinary lock competition on the same exact key

The worker outcome mapping should continue to treat policy rejection as non-retry by default unless future design explicitly changes that contract.

## Observability

Phase 2a should add explicit overlap observability.

Useful signals:

- overlap rejection count by definition
- overlap rejection count by parent definition
- descendant marker publish/remove failures
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

## Testing Requirements

Phase 2a test coverage should include:

- registry rejection for invalid child template shapes
- recursive lineage validation across more than one ancestor level
- runtime parent acquire rejected by active child on another process/backend client
- runtime child acquire rejected by active parent on another process/backend client
- worker parent acquire rejected by active child on another process/backend client
- worker child acquire rejected by active parent on another process/backend client
- descendant marker cleanup on normal release
- descendant marker cleanup on lease expiry
- descendant marker cleanup on renewal failure paths

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
- overlap behavior is reject-first and consistent with composite behavior
- multi-lock operations still require composite plans

## Open Questions Deferred

- whether strict mode should adopt the same lineage model later
- whether a more specific overlap error type should be introduced
- whether future backends should expose lineage-aware capabilities explicitly in the driver contract
- whether presence APIs should eventually surface descendant-active state to application callers
