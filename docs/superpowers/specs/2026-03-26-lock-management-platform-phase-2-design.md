# Lock Management Platform SDK for Go: Phase 2 Design

## Status

Draft

## Relationship to the Base Spec

This document narrows the Phase 2 implementation scope described in [2026-03-26-lock-management-platform-design.md](/Users/mrt/workspaces/boilerplate/lockman/docs/superpowers/specs/2026-03-26-lock-management-platform-design.md).

The base spec remains the source of truth for platform vocabulary, long-term goals, and package boundaries. This Phase 2 design adds implementation-focused decisions for:

- async worker claim execution
- the first production driver
- idempotency contracts
- child and composite standard-mode support
- registry validation hardening required to support the above

This document does not expand scope beyond Phase 2. In particular, it does not introduce public strict-mode execution, fencing-token enforcement, guarded persistence writes, or strict composite runtime behavior.

## Phase 2 Scope

Phase 2 will implement the following from the base spec adoption plan:

- add `workers`
- ship the first production driver
- add idempotency contracts
- add child and composite locks
- harden registry validation

Phase 2 design choices already decided:

- `Redis` is the first production driver
- worker runtime is queue-agnostic
- async outcomes are normalized to `ack`, `retry`, `drop`, or `dlq`
- `IdempotencyStore` is a required interface boundary even when Redis is the first implementation
- parent-child overlap default behavior is `reject`
- composite support in Phase 2 is limited to `standard` mode

## Non-Goals

- No public strict-mode worker execution
- No fencing token issuance or guarded persistence writes
- No repository helper contracts for strict-mode paths
- No auto-escalation from child lock to parent lock
- No backend-specific worker middleware for Kafka, SQS, RabbitMQ, or any single queue product

## Core Phase 2 Decisions

### Worker-First Delivery Model

Phase 2 is worker-first rather than driver-first.

The worker claim lifecycle is the main orchestration path that binds together:

- Redis claim semantics
- idempotency state transitions
- async outcome mapping
- child and composite lock validation

This prevents the Redis driver contract from becoming more abstract than necessary while still preserving backend isolation.

### Redis-First, Interface-First

Redis is the first production backend, but worker runtime and idempotency runtime must remain interface-driven.

Phase 2 should therefore ship:

- a Redis lock driver implementation
- an `IdempotencyStore` contract
- a Redis-backed implementation of that contract

Worker code may depend on the interfaces, but not on Redis-specific types.

### Reject-First Overlap Policy

When a parent lock and child lock overlap in the same resource tree, runtime rejects the request by default.

Phase 2 does not support implicit escalation. If escalation is ever added later, it must be introduced as an explicit policy with clear registry semantics rather than inferred at runtime.

### Standard Composite Only

Phase 2 supports composite locking only for `standard` mode.

The public contract may preserve forward-compatible type shapes for future strict composite support, but runtime behavior, validation, and examples in Phase 2 must treat strict composite execution as out of scope.

## Package Boundaries

Phase 2 extends the package layout from the base spec with these responsibilities:

### `lockkit/workers`

Owns async claim orchestration.

Responsibilities:

- `ExecuteClaimed`
- `ExecuteCompositeClaimed`
- worker claim lifecycle
- claim-context construction
- async outcome mapping
- shutdown integration for worker execution

`workers` must not embed Redis details directly.

### `lockkit/idempotency`

Owns idempotency contracts and state model.

Responsibilities:

- `IdempotencyStore` interface
- record and status types
- begin, complete, and fail transitions
- queue-agnostic state semantics

### `lockkit/idempotency/redis`

Owns the first concrete Redis-backed idempotency implementation.

Responsibilities:

- atomic begin semantics
- TTL-backed record expiry
- safe complete and fail transitions
- owner and message metadata storage for diagnostics

### `lockkit/drivers`

Continues to own backend adapter abstraction for coordination primitives.

Phase 2 may extend the contract where needed for worker claims and metadata inspection, but coordination policy must remain outside the driver.

### `lockkit/drivers/redis`

Owns the first production lock driver.

Responsibilities:

- single-resource claim acquire
- renew
- release
- check presence
- ping or health
- owner and expiry inspection
- standard-mode composite member acquisition support through the existing driver contract plus runtime ordering logic

### `lockkit/runtime`

Extends sync execution to cover standard composite execution.

Phase 2 runtime responsibilities:

- `ExecuteCompositeExclusive`
- canonical member ordering
- rollback on partial composite failure
- overlap rejection

Sync runtime must remain distinct from worker orchestration.

### `lockkit/definitions`

Extends definition and request shapes for:

- child locks
- composite locks
- async claim requests
- claim contexts
- overlap and ordering enums needed for validation

### `lockkit/registry`

Extends registration and validation for child and composite definitions.

### `lockkit/internal/policy`

Owns Phase 2 policy logic that should not live in drivers:

- canonical composite ordering
- overlap rejection logic
- runtime override narrowing
- async outcome mapping helpers

## API and Contract Decisions

### Worker Lock Manager

Phase 2 should implement the async API shape already described in the base spec:

```go
type WorkerLockManager interface {
    ExecuteClaimed(
        ctx context.Context,
        req definitions.MessageClaimRequest,
        fn func(ctx context.Context, claim definitions.ClaimContext) error,
    ) error

    ExecuteCompositeClaimed(
        ctx context.Context,
        req definitions.CompositeClaimRequest,
        fn func(ctx context.Context, claim definitions.ClaimContext) error,
    ) error
}
```

### Worker Outcome

Phase 2 should make worker outcome explicit and queue-agnostic:

```go
type WorkerOutcome string

const (
    OutcomeAck   WorkerOutcome = "ack"
    OutcomeRetry WorkerOutcome = "retry"
    OutcomeDrop  WorkerOutcome = "drop"
    OutcomeDLQ   WorkerOutcome = "dlq"
)
```

Worker callbacks should return business errors. Outcome mapping should remain a runtime concern rather than being hand-coded in each consumer.

`WorkerOutcome` remains an internal runtime model in Phase 2 even if the public API still returns `error` as defined by the base spec. The runtime may use `WorkerOutcome` internally for integration adapters, middleware, and tests, but Phase 2 should not silently fork the public contract away from the source-of-truth spec.

### Idempotency Store

Phase 2 introduces a first-class interface contract:

```go
type Status string

const (
    StatusMissing    Status = "missing"
    StatusInProgress Status = "in_progress"
    StatusCompleted  Status = "completed"
    StatusFailed     Status = "failed"
)

type Record struct {
    Key       string
    Status    Status
    OwnerID   string
    MessageID string
    UpdatedAt time.Time
    ExpiresAt time.Time
}

type BeginInput struct {
    OwnerID      string
    MessageID    string
    ConsumerGroup string
    Attempt      int
    TTL          time.Duration
}

type BeginResult struct {
    Record    Record
    Acquired  bool
    Duplicate bool
}

type CompleteInput struct {
    OwnerID   string
    MessageID string
    TTL       time.Duration
}

type FailInput struct {
    OwnerID   string
    MessageID string
    TTL       time.Duration
}

type Store interface {
    Get(ctx context.Context, key string) (Record, error)
    Begin(ctx context.Context, key string, input BeginInput) (BeginResult, error)
    Complete(ctx context.Context, key string, input CompleteInput) error
    Fail(ctx context.Context, key string, input FailInput) error
}
```

Required behavior:

- `Begin` must be atomic
- duplicate begin attempts must not allow multiple active workers to believe they own the same idempotency key
- terminal completion must be queryable on later retries
- the store must carry enough metadata for observability and operational debugging
- `BeginResult` must let worker runtime distinguish a newly acquired processing slot from an already-terminal duplicate

Phase 2 runtime depends only on `Store`, not on Redis implementation types.

## Worker Lifecycle

Phase 2 worker execution should follow this lifecycle:

1. Resolve lock definition from the registry.
2. Build the resource key, or composite member keys.
3. Validate ownership and worker metadata.
4. Validate idempotency requirements declared by the definition.
5. Validate overlap and composite policy.
6. Consult `IdempotencyStore`.
7. Acquire the claim through the driver.
8. Construct `ClaimContext`.
9. Execute the worker callback.
10. Map the result to `ack`, `retry`, `drop`, or `dlq`.
11. Persist idempotency terminal state when required.
12. Release the claim.

Important sequencing rules:

- idempotency state transitions must occur through the interface contract, not via Redis calls inside worker middleware
- runtime must not report `ack` before required idempotency terminal state is persisted
- release must remain best-effort in shutdown and cancellation paths

## Idempotency Semantics

Phase 2 idempotency semantics are intentionally narrower than eventual strict-mode semantics.

Recommended handling:

- `missing`: normal processing path
- `in_progress`: default to `retry`
- `completed`: default to `ack`
- `failed`: default to `retry`, subject to policy

Phase 2 does not attempt exactly-once guarantees. It provides deterministic coordination hooks that reduce duplicate work and allow worker middleware to make explicit retry decisions.

## Redis Driver Design

### Lock Claim Semantics

The Redis driver should implement claim semantics using TTL-backed ownership records.

Recommended single-resource behavior:

- acquire: atomic create-if-absent with TTL
- renew: owner-checked TTL extension
- release: owner-checked delete
- presence: inspect owner and expiry metadata

Ownership checks must be compare-and-set style operations rather than blind overwrites or deletes.

### Composite Standard Semantics

Composite execution remains a runtime concern rather than a driver concern.

Runtime should:

1. sort members using canonical ordering
2. acquire claims one by one
3. roll back in reverse order on failure

The Redis driver only needs to support the underlying single-member lease operations consistently.

### Redis Implementation Notes

Phase 2 Redis design should assume atomic script-backed operations for at least:

- owner-checked release
- owner-checked renew
- any metadata inspection path that must read consistent owner and expiry state

Simple `SET NX PX` plus follow-up best-effort deletes is not sufficient on its own for safe owner-checked release behavior.

## Child and Composite Support

### Child Locks

Phase 2 adds child lock support where the child invariant is independent from sibling children.

Requirements:

- child definitions must declare `ParentRef`
- `ParentRef` must resolve to a known definition
- overlap with parent in the same execution tree defaults to `reject`

### Composite Locks

Phase 2 adds declared composite plans for standard mode only.

Requirements:

- all members must refer to known definitions
- all members must resolve to `standard` mode
- ordering must be deterministic
- member count must respect configured cap
- partial acquisition failure must release already acquired members in reverse order

## Registry Validation Additions

Phase 2 must extend validation beyond Phase 1 to cover:

- child locks referencing unknown parents
- composite definitions referencing unknown members
- invalid or incomplete ordering metadata
- strict async definitions that do not require idempotency
- composite plans mixing modes
- composite plans exceeding member limit
- invalid parent-child overlap policy values
- unsupported backend failure policy values

Additionally, because Phase 2 adopts Redis as the first production backend, startup should include a separate backend-compatibility check for the selected driver. That compatibility check may reject definitions that imply behavior the Redis-backed standard path cannot safely honor, but those checks should not redefine the backend-agnostic registry invariants themselves.

## Outcome Mapping Policy

Phase 2 worker runtime should implement the base spec’s recommended outcome mapping shape:

- `Busy` -> `retry`
- `AcquireTimeout` -> `retry`
- `DuplicateIgnored` -> `ack`
- `InvariantRejected` -> `ack` or `drop` by policy
- infrastructure failure -> `retry` or `dlq` by policy

The exact mapping implementation should live in policy helpers, not inside driver implementations and not inside business callbacks.

## Testing Expectations

Phase 2 should add:

- worker lifecycle unit tests
- composite ordering and rollback tests
- child overlap rejection tests
- Redis driver integration tests
- Redis-backed idempotency store tests
- shutdown tests that cover in-flight worker claims and renew/release cleanup

The first production driver should not be considered complete without integration-style verification against a real Redis instance in CI or controlled local test harnesses.

## Deliverables

A complete Phase 2 implementation should produce:

- `lockkit/workers`
- `lockkit/idempotency`
- a Redis-backed `IdempotencyStore` implementation
- a Redis production lock driver
- standard composite sync execution
- standard composite worker execution
- child-lock validation and overlap rejection
- hardened registry validation for Phase 2 definition shapes

## Explicit Deferrals to Later Phases

Still deferred after Phase 2:

- strict runtime execution
- fencing tokens
- guarded write contracts
- repository helper contracts for strict-mode persistence
- strict composite runtime behavior
- full worker policy standardization beyond the baseline mapping above

## Acceptance Criteria

Phase 2 is complete when:

- worker claim execution runs through a queue-agnostic API
- idempotency is enforced through an interface boundary
- Redis is usable as the first production backend
- child locks validate and reject invalid overlap cases
- standard composite execution works for sync and worker paths
- validation rules prevent unsupported Phase 2 definitions from reaching runtime
