# Lock Management Platform SDK for Go

## Status

Draft

## Context

The system has concurrent read/write paths across sync request handlers and async queue workers. Locking is currently treated as a local implementation detail, which leads to inconsistent lock naming, duplicated acquire/release patterns, unclear ownership, and weak safety guarantees for async retries and duplicate messages.

As the number of locks grows, the system needs a single architecture-level solution that:

- manages lock definitions centrally
- supports multiple lock backends without coupling application code to one storage
- distinguishes lightweight coordination from strict critical-path coordination
- remains usable for both sync application flows and async queue workers
- exposes observability and governance so locks do not become operationally opaque

## Problem Statement

Build a Go library that acts as a coordination platform SDK rather than a thin distributed mutex wrapper. The library must let teams define, enforce, observe, and evolve locking policy centrally while keeping business code readable and avoiding lock sprawl.

## Goals

- Enforce a central lock registry for all production lock usage.
- Support parent, child, and composite lock types.
- Support check-only lock presence queries without acquisition.
- Support both sync execution and async queue-worker claim flows.
- Support `standard` and `strict` coordination modes.
- Provide a backend-agnostic driver abstraction.
- Provide a persistence safety contract for strict mode.
- Provide built-in metrics, tracing, audit hooks, and typed outcomes.

## Non-Goals

- This is not a workflow or saga engine.
- This does not guarantee strong correctness if persistence writes ignore guard metadata.
- This does not expose raw lock primitives as the default application integration path.

## Core Decisions

### Platform Shape

The solution is a `Lock Management Platform SDK`, not a storage-specific locking client.

### Coordination Modes

The library supports a mixed model:

- `standard`: pragmatic mutual exclusion for non-critical flows
- `strict`: lease-based coordination with fencing and guarded persistence writes

### Governance Model

All lock definitions are declared in a central registry. Application code may reference predefined definitions but may not construct ad hoc lock names or policies at runtime.

### Async Model

Queue workers process independent messages. Locks are not held across message boundaries. Each worker claims coordination rights when it starts processing the message.

## Lock Taxonomy

The SDK must support all three lock types. They are complementary, not interchangeable.

### Parent Lock

A parent lock protects an aggregate or business root.

Examples:

- `order:123`
- `user:42`
- `invoice:abc`

Use a parent lock when the protected invariant lives at the aggregate level, even if the code path touches sub-resources.

### Child Lock

A child lock protects a sub-resource with independent correctness semantics.

Examples:

- `order:123:item:1`
- `file:abc:chunk:02`
- `campaign:9:recipient:42`

Use a child lock only when concurrent updates to sibling children do not break parent-level correctness.

### Composite Lock

A composite lock is a declared multi-resource acquisition plan for operations that require multiple peer resources at once.

Examples:

- transfer between `account:A` and `account:B`
- reserve both `inventory:sku1` and `coupon:xyz`

Composite acquisition is the only approved multi-lock pattern. Nested ad hoc lock acquisition in application code is disallowed.

## Lock Presence Check

The platform should also support a check-only interaction that asks whether a lock is currently held without attempting to acquire it.

This is not a fourth lock taxonomy beside parent, child, and composite. It is an access pattern that can be applied to an existing lock definition.

Example use cases:

- reject a UI action if a resource is already being processed
- show operational status for a resource in an admin screen
- perform soft gating before enqueueing duplicate work

Presence checks are advisory only. They must not be used as a correctness guarantee for write-critical operations because they are vulnerable to time-of-check versus time-of-use races.

## Governance Rules

- Every production lock must be defined in the central registry.
- Every multi-lock flow must be defined as a composite lock plan.
- Application code must not acquire by raw string key.
- Runtime overrides may narrow behavior but must not break registry policy.
- Nested lock acquisition is rejected unless it goes through a declared composite plan.
- Startup fails if the registry is invalid.
- Lock ownership is non-reentrant by default and same-owner re-acquisition is rejected.

## Registry Model

### LockDefinition

Each lock definition should include at minimum:

- `ID`
- `Kind` (`parent` or `child`)
- `Resource`
- `Mode` (`standard` or `strict`)
- `ExecutionKind` (`sync`, `async`, or `both`)
- `LeaseTTL`
- `WaitTimeout`
- `RetryPolicy`
- `BackendFailurePolicy`
- `FencingRequired`
- `IdempotencyRequired`
- `Rank`
- `ParentRef`
- `KeyBuilder`
- `CheckOnlyAllowed`
- `Tags`

### CompositeDefinition

Each composite definition should include at minimum:

- `ID`
- `Members`
- `OrderingPolicy`
- `AcquirePolicy`
- `EscalationPolicy`
- `ModeResolution`
- `MaxMemberCount`
- `ExecutionKind`

Recommended default: cap composite member count at `5` unless a stronger use case justifies a higher limit.

### KeyBuilder

`KeyBuilder` must be deterministic and introspectable by validation logic.

Recommended interface:

```go
type KeyBuilder interface {
    RequiredFields() []string
    Build(input map[string]string) (string, error)
}
```

The SDK may provide template-backed builders such as `order:{order_id}:item:{item_id}`, but validation must still know the required fields and guarantee stable output for identical input.

## Validation Rules

Registry validation must fail fast when:

- lock IDs collide
- child locks refer to unknown parents
- composite members refer to unknown definitions
- ordering metadata is incomplete
- strict locks do not require fencing
- strict async locks do not require idempotency
- non-reentrant policy is overridden by a definition
- composite plans mix `standard` and `strict` members
- composite plans exceed configured member limits
- key builders cannot build deterministic keys from required input
- invalid parent-child overlap policy is declared
- unsupported backend failure policy is declared

## Architecture

Recommended package layout:

```text
lockkit/
  definitions/
  registry/
  runtime/
  workers/
  guard/
  integration/
  testing/
  drivers/
  observe/
  errors/
  internal/policy/
```

Responsibilities:

- `definitions`: lock definitions, composite definitions, enums
- `registry`: registration, validation, lookup
- `runtime`: sync execution lifecycle
- `workers`: async claim lifecycle and outcome mapping
- `guard`: persistence safety contracts
- `integration`: decorators, middleware, and repository-facing helper contracts
- `testing`: in-memory driver, relaxed test registry, and assertion helpers
- `drivers`: backend adapter abstraction
- `observe`: metrics, tracing, audit hooks
- `errors`: typed runtime errors and write outcomes

`registry` owns definition storage and validation. `internal/policy` owns runtime rule evaluation such as override narrowing, overlap handling, degradation behavior, and context-deadline reconciliation.

Minimum testing support:

- `InMemoryDriver` implementing the driver contract
- `TestRegistry` with intentionally lighter setup friction for unit tests
- assertion helpers for acquired, released, and lease-lost states

## Ownership and Execution Metadata

Every execution must carry normalized ownership metadata so runtime, workers, and observability use the same vocabulary.

Minimum metadata:

- `service_name`
- `instance_id`
- `handler_name`
- `owner_id`
- `request_id` for sync paths
- `message_id` and `attempt` for async paths
- `consumer_group` for async worker paths

This metadata must be attached to lock acquisition, guard context, metrics, traces, and audit events.

## Runtime APIs

### Minimum Request and Context Shapes

The exact implementation may evolve, but the minimum request and context shapes must be explicit.

```go
type OwnershipMeta struct {
    ServiceName   string
    InstanceID    string
    HandlerName   string
    OwnerID       string
    RequestID     string
    MessageID     string
    Attempt       int
    ConsumerGroup string
}

type RuntimeOverrides struct {
    WaitTimeout *time.Duration
    MaxRetries  *int
}

type SyncLockRequest struct {
    DefinitionID string
    KeyInput     map[string]string
    Ownership    OwnershipMeta
    Overrides    *RuntimeOverrides
}

type MessageClaimRequest struct {
    DefinitionID   string
    KeyInput       map[string]string
    Ownership      OwnershipMeta
    IdempotencyKey string
    Overrides      *RuntimeOverrides
}

type CompositeLockRequest struct {
    DefinitionID string
    MemberInputs []map[string]string
    Ownership    OwnershipMeta
    Overrides    *RuntimeOverrides
}

type CompositeClaimRequest struct {
    DefinitionID   string
    MemberInputs   []map[string]string
    Ownership      OwnershipMeta
    IdempotencyKey string
    Overrides      *RuntimeOverrides
}

type PresenceCheckRequest struct {
    DefinitionID string
    KeyInput     map[string]string
    Ownership    OwnershipMeta
}

type LeaseContext struct {
    DefinitionID  string
    ResourceKey   string
    ResourceKeys  []string
    Ownership     OwnershipMeta
    FencingToken  uint64
    LeaseTTL      time.Duration
    LeaseDeadline time.Time
}

type ClaimContext struct {
    DefinitionID   string
    ResourceKey    string
    ResourceKeys   []string
    Ownership      OwnershipMeta
    FencingToken   uint64
    LeaseTTL       time.Duration
    LeaseDeadline  time.Time
    IdempotencyKey string
}

type PresenceState int

const (
    PresenceHeld PresenceState = iota
    PresenceNotHeld
    PresenceUnknown
)

type PresenceStatus struct {
    State         PresenceState
    Mode          string
    OwnerID       string
    LeaseDeadline time.Time
}
```

Context field conventions:

- `ResourceKey` is populated for single-resource execution
- `ResourceKeys` is populated for composite execution
- single-resource execution may leave `ResourceKeys` empty
- composite execution may leave `ResourceKey` empty

Override rules:

- Allowed narrowing: lower `WaitTimeout`, lower retry count
- Disallowed widening: higher `LeaseTTL`, mode changes, fencing changes, idempotency changes
- Unknown or unsupported overrides must be rejected at runtime

### Sync API

```go
type LockManager interface {
    ExecuteExclusive(
        ctx context.Context,
        req SyncLockRequest,
        fn func(ctx context.Context, lease LeaseContext) error,
    ) error

    ExecuteCompositeExclusive(
        ctx context.Context,
        req CompositeLockRequest,
        fn func(ctx context.Context, lease LeaseContext) error,
    ) error
}
```

Use for request-scoped or command-scoped sync execution.

Recommended integration path:

- command handler decorator
- HTTP or RPC middleware
- application-service wrapper

### Presence Check API

```go
type LockInspector interface {
    CheckPresence(
        ctx context.Context,
        req PresenceCheckRequest,
    ) (PresenceStatus, error)
}
```

Recommended semantics:

- resolve definition from the registry
- build the resource key
- query whether the lock is currently held
- return advisory metadata such as owner, lease expiry, and mode when available

Recommended integration path:

- UI or API pre-checks
- enqueue dedup pre-checks
- admin or operational inspection endpoints

When the backend is unavailable or health is unknown, `CheckPresence` should return `PresenceUnknown` rather than collapsing the result into held or not-held.

### Async Worker API

```go
type WorkerLockManager interface {
    ExecuteClaimed(
        ctx context.Context,
        req MessageClaimRequest,
        fn func(ctx context.Context, claim ClaimContext) error,
    ) error

    ExecuteCompositeClaimed(
        ctx context.Context,
        req CompositeClaimRequest,
        fn func(ctx context.Context, claim ClaimContext) error,
    ) error
}
```

Use for queue workers processing one message at a time.

Recommended integration path:

- worker middleware
- consumer decorator
- job-runner wrapper

### Restricted Low-Level API

Low-level acquire, renew, and release primitives may exist internally or for exceptional cases, but they are not the golden path.

### Lifecycle Management API

Because the SDK may own background renewal goroutines and driver resources, managers should expose graceful shutdown.

```go
type Lifecycle interface {
    Shutdown(ctx context.Context) error
}
```

## Execution Lifecycle

### Sync Lifecycle

1. Resolve lock definition from the registry.
2. Build the resource key.
3. Validate the request against policy.
4. Acquire the lock.
5. Issue fencing metadata when strict mode applies.
6. Execute the callback.
7. Release the lock.
8. Emit metrics, tracing, and audit events.

Renewal strategy:

- The SDK owns lease renewal, not application code.
- Renewal starts automatically for executions that outlive one renewal interval.
- The default renewal interval is `LeaseTTL / 3`, subject to implementation-level min and max safety clamps.
- Renewal failure marks the lease as lost, cancels the callback context, emits `ErrLeaseLost`, and triggers best-effort cleanup or release.

### Async Worker Lifecycle

1. Resolve lock definition from the registry.
2. Build the resource key from message payload.
3. Validate message metadata, including idempotency when required.
4. Acquire the claim.
5. Issue fencing metadata when strict mode applies.
6. Execute the worker callback.
7. Perform guarded persistence writes when strict mode applies.
8. Map result to `ack`, `retry`, `drop`, or `dlq`.
9. Release the claim.
10. Emit metrics, tracing, and audit events.

Renewal follows the same SDK-owned model as sync execution. Worker handlers must not own lease renewal directly.

Composite execution is supported for async workers through `ExecuteCompositeClaimed`. Async composite use should remain rare and short-lived because contention and failure handling complexity grow quickly with each additional member.

### Presence Check Lifecycle

1. Resolve lock definition from the registry.
2. Build the resource key.
3. Validate that the definition allows check-only access.
4. Query current lock state from the driver.
5. Return advisory presence metadata.
6. Emit metrics and tracing events.

## Context and Deadline Semantics

- Caller `context.Context` deadline always wins over configured `WaitTimeout`.
- Effective acquire timeout is `min(context deadline, WaitTimeout)`.
- Context cancellation stops waiting immediately.
- Once a lock is acquired, context cancellation triggers best-effort release rather than waiting for TTL expiry.
- `LeaseTTL` remains policy-defined and cannot be widened by runtime overrides.
- Long-running handlers should keep the critical section short even if SDK renewal is available.

## Fairness and Waiter Ordering

The SDK does not guarantee FIFO fairness by default.

- waiter ordering is driver-dependent
- best-effort contention handling is acceptable unless a driver explicitly documents stronger guarantees
- applications must not assume starvation freedom unless the selected driver provides it

## Composite and Overlap Rules

- Composite lock plans are all-or-nothing.
- Runtime acquires composite members using canonical ordering.
- When parent and child locks overlap within the same resource tree, runtime either escalates to parent or rejects, based on declared policy.
- Parent lock is the default recommendation for aggregate-critical write paths.
- Partial composite acquisition failure triggers rollback by releasing already acquired members in reverse acquisition order.
- Composite plans may overlap with other composite plans as long as canonical ordering remains global and deterministic.

Canonical ordering should be deterministic across the entire system. The recommended default is:

1. sort by lock rank
2. sort by resource type
3. sort by normalized resource key

Application code must not choose acquisition order dynamically.

Mixed-mode composite plans are invalid. A composite definition must contain either all `standard` members or all `strict` members.

For strict composite execution, fencing semantics are composite-scoped rather than member-scoped. A successful composite acquisition yields one execution-level fencing token that must flow through guarded writes for the whole critical section. If future drivers or persistence models require per-member fencing, that behavior must be introduced explicitly as a later design change rather than inferred.

## Driver Abstraction

Drivers are backend adapters, not policy owners. A driver is responsible for:

- acquire
- renew
- release
- check presence
- ping or health check
- inspect lease ownership and token metadata

Coordination semantics remain in runtime, worker, and guard layers rather than in storage-specific drivers.

## Graceful Degradation Policy

Backend outage behavior must be explicit rather than inferred.

Recommended defaults:

- `standard` mode: fail closed by default; an explicit `best_effort_open` policy may be allowed for low-risk paths
- `strict` mode: always fail closed
- presence check: return `Unknown` status and an error, never pretend the resource is unlocked

`BackendFailurePolicy` must be declared per definition and validated against mode constraints. `strict` definitions may not opt into fail-open behavior.

## Reentrancy Policy

The platform is non-reentrant.

If the same owner attempts to acquire the same lock again within the same process or execution tree, runtime must reject the request with a typed policy error rather than deadlocking or reference-counting implicitly.

## Shutdown and Cleanup Semantics

During graceful shutdown, the SDK should:

- stop accepting new acquisitions
- cancel renewal loops
- attempt best-effort release of locally held locks
- fall back to lease expiry if a clean release cannot be completed before shutdown deadline

`Shutdown(ctx)` should be idempotent and safe to call during process termination.

## Strict Mode Contract

Strict mode is only meaningful when all of the following are present:

- distributed lease or claim
- fencing token
- idempotency protection for async flows
- guarded write at persistence time

Strict mode should not be marketed or documented as strong coordination without persistence guard integration.

### Guard Context

```go
type GuardContext struct {
    LockID         string
    ResourceKey    string
    FencingToken   uint64
    OwnerID        string
    MessageID      string
    IdempotencyKey string
}
```

Repository code on strict paths must accept guard metadata, not only business payload.

Recommended bridge helpers:

```go
func GuardContextFromLease(lease LeaseContext) GuardContext
func GuardContextFromClaim(claim ClaimContext) GuardContext
```

These helpers should preserve lock ID, resource key, fencing token, owner identity, and idempotency metadata when available.

## Persistence Safety Guideline

Strict resources should expose storage semantics equivalent to:

- `version`
- `last_fencing_token`
- `idempotency_key` or a deduplication store
- `updated_by_owner`
- `updated_at`

Guarded write outcomes should be normalized to:

- `Applied`
- `DuplicateIgnored`
- `StaleRejected`
- `VersionConflict`
- `InvariantRejected`

These outcomes allow worker runtime to make deterministic ack or retry decisions.

## Async Outcome Mapping

Recommended default mappings:

- `Busy` -> retry later
- `AcquireTimeout` -> retry
- `DuplicateIgnored` -> ack
- `StaleRejected` -> ack or safe drop
- `VersionConflict` -> retry or reject by policy
- `InvariantRejected` -> ack with business logging
- `InfrastructureError` -> retry or dlq by policy

Outcome mapping should be configurable through registry policy rather than duplicated inside each worker handler.

## Integration Layer

The SDK should ship first-class boundary adapters so application code does not repeatedly rebuild lock orchestration.

Recommended integration components:

- sync handler decorator wrapping `ExecuteExclusive`
- async worker middleware wrapping `ExecuteClaimed`
- presence-check helper wrapping `CheckPresence`
- repository helper types for strict guarded writes
- idempotency helper contract for async consumers

The golden path is integration at the application boundary. Direct low-level lock orchestration inside business code should be treated as an exception.

## Registry Loading Model

The registry should be programmatic-first for Go adoption.

- primary path: Go code registers definitions during service startup
- optional future path: declarative loaders may exist for generated or shared definitions

Programmatic registration keeps type safety, discoverability, and refactoring support aligned with normal Go development.

## Observability

Observability should be OpenTelemetry-first. The SDK may provide adapters or examples for Prometheus export, but its primary instrumentation contract should align with OpenTelemetry metrics and tracing.

### Metrics

Minimum metrics:

- acquire latency
- presence-check latency
- wait time
- contention count
- timeout count
- renew count
- lease lost count
- active lock count
- hold duration
- duplicate count
- stale reject count
- guarded write latency
- guarded write outcome count
- composite member count

Minimum tags:

- `lock_id`
- `resource_type`
- `mode`
- `execution_kind`
- `service`
- `handler`
- `consumer_group`

### Tracing

Recommended spans:

- `lock.execution`
- `lock.resolve`
- `lock.acquire`
- `lock.renew`
- `handler.execute`
- `guarded_write`
- `lock.release`

Recommended hierarchy:

```text
lock.execution
├── lock.resolve
├── lock.acquire
├── handler.execute
│   └── guarded_write
├── lock.renew (0..N)
└── lock.release
```

### Audit and Introspection

Operators should be able to inspect:

- current lock holder
- service and instance ownership
- lease expiry
- fencing token
- recent contention behavior

### Audit Hook

Recommended interface:

```go
type AuditHook interface {
    OnAcquire(ctx context.Context, event AcquireEvent)
    OnRelease(ctx context.Context, event ReleaseEvent)
    OnGuardedWrite(ctx context.Context, event GuardWriteEvent)
    OnContention(ctx context.Context, event ContentionEvent)
}
```

Audit hooks should be non-blocking with respect to lock lifecycle. The recommended implementation is asynchronous dispatch or buffered fire-and-forget delivery. Hook failures must not prevent release, renewal cleanup, or guarded-write completion.

## Error and Outcome Taxonomy

Prefer typed errors over bare sentinel errors. Error values should work with `errors.Is` and `errors.As`, and retry-relevant errors should expose stable behavior through typed inspection rather than string matching.

Recommended typed errors and outcomes:

- `ErrLockBusy`
- `ErrLockAcquireTimeout`
- `ErrLeaseLost`
- `ErrStaleToken`
- `ErrDuplicateMessage`
- `ErrIdempotencyConflict`
- `ErrRegistryViolation`
- `ErrPolicyViolation`
- `ErrReentrantAcquire`
- `ErrGuardRejected`

This taxonomy is required so application code and worker runtimes can distinguish retryable conditions from safe terminal outcomes.

## Versioning and Compatibility

The SDK should follow semantic versioning.

- breaking API or behavioral changes require a major version bump
- deprecated definitions and policy fields should remain supported for at least one minor release before removal
- registry schema changes should be versioned explicitly if declarative loading is introduced later

Compatibility notes must call out changes that alter locking behavior, not just type signatures.

## Adoption Plan

### Phase 1

- Implement `definitions`, `registry`, and `errors`.
- Implement `testing` with `InMemoryDriver`.
- Support `standard` mode.
- Support parent locks.
- Add baseline metrics.

### Phase 2

- Add `workers`.
- Ship the first production driver.
- Add idempotency contracts.
- Add child and composite locks.
- Harden registry validation.

### Phase 3a

- Add `strict` mode.
- Add fencing token support.

### Phase 3b

- Add guarded write contracts.
- Add repository helper contracts.

### Phase 3c

- Add tracing, audit hooks, and introspection.

### Phase 4

- Standardize worker retry and ack policy.
- Harden runtime enforcement.
- Add production readiness validation and operational docs.

## Decision Summary

- Build a lock management platform SDK, not a backend-specific lock helper.
- Enforce central registry usage.
- Support parent, child, and composite lock taxonomies together.
- Prefer parent lock for aggregate-critical write paths.
- Use separate sync and async APIs.
- Use claim-based async coordination for queue workers.
- Treat strict mode as `lease + fencing + idempotency + guarded persistence write`.

## Open Questions

- Which driver should be implemented first for internal adoption?
- Should guarded write helpers be generic interfaces or opinionated repository adapters?
- Should parent-child overlap default to escalation or explicit rejection?
- What is the operational migration strategy if a service changes lock backend drivers?
- What clock-skew assumptions are acceptable for lease inspection and operator-facing expiry visibility?
