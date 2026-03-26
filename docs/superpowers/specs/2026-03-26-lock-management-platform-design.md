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
- `ExecutionKind`

## Validation Rules

Registry validation must fail fast when:

- lock IDs collide
- child locks refer to unknown parents
- composite members refer to unknown definitions
- ordering metadata is incomplete
- strict locks do not require fencing
- strict async locks do not require idempotency
- key builders cannot build deterministic keys from required input
- invalid parent-child overlap policy is declared

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
- `drivers`: backend adapter abstraction
- `observe`: metrics, tracing, audit hooks
- `errors`: typed runtime errors and write outcomes

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

### Sync API

```go
type LockManager interface {
    ExecuteExclusive(
        ctx context.Context,
        req SyncLockRequest,
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

### Async Worker API

```go
type WorkerLockManager interface {
    ExecuteClaimed(
        ctx context.Context,
        req MessageClaimRequest,
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

### Presence Check Lifecycle

1. Resolve lock definition from the registry.
2. Build the resource key.
3. Validate that the definition allows check-only access.
4. Query current lock state from the driver.
5. Return advisory presence metadata.
6. Emit metrics and tracing events.

## Composite and Overlap Rules

- Composite lock plans are all-or-nothing.
- Runtime acquires composite members using canonical ordering.
- When parent and child locks overlap within the same resource tree, runtime either escalates to parent or rejects, based on declared policy.
- Parent lock is the default recommendation for aggregate-critical write paths.

Canonical ordering should be deterministic across the entire system. The recommended default is:

1. sort by lock rank
2. sort by resource type
3. sort by normalized resource key

Application code must not choose acquisition order dynamically.

## Driver Abstraction

Drivers are backend adapters, not policy owners. A driver is responsible for:

- acquire
- renew
- release
- check presence
- inspect lease ownership and token metadata

Coordination semantics remain in runtime, worker, and guard layers rather than in storage-specific drivers.

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

## Observability

### Metrics

Minimum metrics:

- acquire latency
- presence-check latency
- wait time
- contention count
- timeout count
- renew count
- lease lost count
- duplicate count
- stale reject count
- guarded write outcome count

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

- `lock.resolve`
- `lock.acquire`
- `lock.renew`
- `handler.execute`
- `guarded_write`
- `lock.release`

### Audit and Introspection

Operators should be able to inspect:

- current lock holder
- service and instance ownership
- lease expiry
- fencing token
- recent contention behavior

## Error and Outcome Taxonomy

Recommended typed errors and outcomes:

- `ErrLockBusy`
- `ErrLockAcquireTimeout`
- `ErrLeaseLost`
- `ErrStaleToken`
- `ErrDuplicateMessage`
- `ErrIdempotencyConflict`
- `ErrRegistryViolation`
- `ErrPolicyViolation`
- `ErrGuardRejected`

This taxonomy is required so application code and worker runtimes can distinguish retryable conditions from safe terminal outcomes.

## Adoption Plan

### Phase 1

- Implement `definitions`, `registry`, and `errors`.
- Support `standard` mode.
- Support parent locks.
- Add baseline metrics.

### Phase 2

- Add `workers`.
- Add idempotency contracts.
- Add child and composite locks.
- Harden registry validation.

### Phase 3

- Add `strict` mode.
- Add fencing token support.
- Add guarded write contracts.
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
