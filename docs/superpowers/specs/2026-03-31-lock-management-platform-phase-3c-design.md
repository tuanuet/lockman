# Lock Management Platform Phase 3c Design

## Status

Draft

## Relationship to Existing Specs

This document extends:

- [2026-03-26-lock-management-platform-design.md](/Users/mrt/workspaces/boilerplate/lockman/docs/superpowers/specs/2026-03-26-lock-management-platform-design.md)
- [2026-03-27-lock-management-platform-phase-3a-design.md](/Users/mrt/workspaces/boilerplate/lockman/docs/superpowers/specs/2026-03-27-lock-management-platform-phase-3a-design.md)
- [2026-03-27-lock-management-platform-phase-3b-design.md](/Users/mrt/workspaces/boilerplate/lockman/docs/superpowers/specs/2026-03-27-lock-management-platform-phase-3b-design.md)

The base spec defines observability, tracing, audit, and introspection as part of the long-term platform shape. Phase 3a made strict execution real. Phase 3b made guarded persistence real. Phase 3c now makes runtime behavior inspectable and exportable in a product-shaped way for adopters and operators.

Phase 3c is the observability phase.

## Problem Statement

The current codebase exposes only a narrow internal metric hook through `lockkit/observe.Recorder`, and only on runtime paths. That is enough for package-local tests, but it is not enough for real operators or adopters who need to:

- inspect live runtime and worker activity
- export lifecycle events to telemetry and audit systems
- debug renewals, lease loss, and shutdown behavior
- mount an admin surface without reading engine internals
- keep observability on the stable root SDK path rather than on `lockkit`

Without a dedicated Phase 3c, the platform continues to have working coordination behavior but weak operational visibility.

## Goal

Add a root-level observability and inspection surface that makes lock lifecycle behavior visible without changing core lock semantics.

Phase 3c should make the following true:

- applications can wire lock observability through stable root packages
- runtime, worker, and client lifecycle events are emitted through one public event model
- operators can query current state and recent event history through package APIs and default HTTP handlers
- observability sinks and exporters are best-effort and never block or fail the lock lifecycle
- inspection state is a process-local operational view, not a distributed correctness source
- OpenTelemetry is the primary tracing direction, while the engine stays insulated behind a thin internal bridge

## Scope

### In Scope

- new public root packages for observability and inspection
- a public event model for runtime, worker, and client lifecycle events
- an async dispatcher that fans out events to sinks and exporters
- a default in-memory inspection store with materialized state and recent event history
- package APIs for inspection snapshots, event queries, and subscriptions
- default `net/http` admin handlers for inspection and event streaming
- OTel-first tracing integration at the public package level
- default logging and audit sink patterns
- internal bridge code that translates engine lifecycle into the new event model
- explicit dispatcher shutdown and drain behavior for bounded best-effort flushing
- migration away from direct product reliance on `lockkit/observe.Recorder`
- docs and examples for basic wiring, OTel wiring, and admin HTTP mounting

### Out Of Scope

- durable event store implementations as first-party adapters
- authn/authz policy for admin endpoints
- changing lock outcome behavior, acquire semantics, or retry semantics
- strict lineage-specific inspection rules beyond current execution facts
- strict composite observability semantics beyond current unsupported boundaries
- configurable worker ack or retry policy derived from observability or guarded-write outcomes
- a platform-wide transaction or diagnostics framework

## Current State

Today the repository has:

- a runtime-only `lockkit/observe.Recorder` interface
- no root-level observability package
- no public inspection state package
- no default HTTP admin surface
- no public event stream or subscription model
- no explicit worker idempotency, renewal, shutdown, or lease-loss event export surface

The current `Recorder` shape is intentionally too small for Phase 3c:

```go
type Recorder interface {
    RecordAcquire(ctx context.Context, definitionID string, wait time.Duration, success bool)
    RecordContention(ctx context.Context, definitionID string)
    RecordOverlapRejected(ctx context.Context, definitionID string)
    RecordTimeout(ctx context.Context, definitionID string)
    RecordActiveLocks(ctx context.Context, definitionID string, count int)
    RecordRelease(ctx context.Context, definitionID string, held time.Duration)
    RecordPresenceCheck(ctx context.Context, definitionID string, duration time.Duration)
}
```

That interface should not become the long-term public product surface. It is an engine-oriented metric hook, not a stable SDK observability contract.

## Core Decisions

### Root-Level Product Surface

Phase 3c should expose observability through stable root packages rather than through `lockkit`.

Recommended public packages:

```text
observe/
inspect/
```

Reasoning:

- root packages match the product direction already established by `backend`, `idempotency`, and `guard`
- adopters should not need to import engine packages to wire telemetry or admin inspection
- `lockkit` should remain the engine implementation boundary, not the operator-facing API

### Two-Package Split

Phase 3c should split public surface into:

- `observe`: event emission, sink and exporter contracts, dispatcher wiring, tracing bridge
- `inspect`: state materialization, recent history, subscriptions, and HTTP handlers

This keeps concerns clear:

- `observe` is append-only and export-oriented
- `inspect` is query-oriented and current-state oriented

Phase 3c should not collapse both into one broad package because that would blur emission contracts and query/state behavior.

### Internal Bridge, Not Direct Engine Exposure

Runtime, worker, and client code should emit through a thin internal bridge that translates engine lifecycle into public `observe.Event` records.

Recommended shape:

```text
lockkit/internal/observebridge/
```

Responsibilities:

- receive lifecycle callbacks from runtime, workers, and client shutdown paths
- normalize fields into the root event model
- keep engine-specific details out of public APIs

The engine must not expose raw manager internals directly to `inspect` HTTP handlers.

### Best-Effort, Non-Blocking Delivery

Observability in Phase 3c is always best-effort.

Hard rule:

- failure to publish, store, export, trace, or audit an event must never fail `Run`, `Claim`, acquire, renew, release, idempotency transitions, or shutdown cleanup

Implications:

- publishing must be non-blocking on the lock lifecycle path
- full buffers must drop according to explicit policy
- slow subscribers or exporters must be isolated from the publisher
- release and renewal cleanup must never wait on telemetry

This rule applies differently to the two public surfaces:

- `inspect` current-state updates may happen synchronously inside the local process because they are in-memory bookkeeping for the same process and do not cross a network or durable boundary
- `observe` export delivery must remain asynchronous and best-effort

That distinction keeps local state accurate enough for admin inspection without letting remote exporters slow the lock path.

To keep that distinction honest, the hot-path ingestion contract must stay narrow:

- synchronous inspection ingestion must be in-memory only
- it must not perform network I/O, disk I/O, tracing export, logging export, or durable writes
- it must not block on downstream subscribers
- it should remain constant-time or near-constant-time with respect to current state size

### OTel-First Tracing With A Thin Generic Bridge

Phase 3c should be OpenTelemetry-first at the public package level.

Recommended direction:

- provide OTel-oriented constructors or adapters from `observe`
- keep a thin generic sink abstraction so the engine does not depend directly on OTel packages

This preserves a stable engine boundary while still aligning public observability with the platform design's OTel-first intent.

### Inspection Includes Current State And Recent History

Phase 3c inspection should include both:

- current materialized state
- recent event history

Current state answers:

- what is active right now?
- what renewals are running?
- is shutdown in progress?

Recent history answers:

- what just happened to this lock?
- was there contention, overlap rejection, renew failure, or lease loss?

Phase 3c should not force operators to reconstruct state from raw exported events only.

However, Phase 3c inspection must be documented honestly:

- it is process-local, not cluster-global
- it is operationally useful, not a correctness boundary
- it may disappear on process restart
- it should not be used to decide whether a write is safe

## Package Design

### `observe`

This package owns event emission and export.

Recommended public shapes:

```go
package observe

type Event struct {
    Kind          EventKind
    Time          time.Time
    LockID        string
    ResourceKey   string
    ResourceKeys  []string
    Mode          string
    ExecutionKind string
    Path          string
    OwnerID       string
    Service       string
    Instance      string
    Handler       string
    RequestID     string
    MessageID     string
    ConsumerGroup string
    Attempt       int
    IdempotencyKey string
    Wait          time.Duration
    Held          time.Duration
    LeaseTTL      time.Duration
    LeaseDeadline time.Time
    FencingToken  uint64
    Outcome       string
    Error         string
    Attributes    map[string]string
}

type Sink interface {
    Consume(context.Context, Event) error
}

type Exporter interface {
    Export(context.Context, []Event) error
}

type Dispatcher interface {
    Publish(Event)
    Shutdown(context.Context) error
}
```

The exact field names may adjust, but the shape must support:

- sync and async execution
- strict and standard execution
- single and composite locks
- idempotency and worker metadata
- timing and outcome metadata
- exporter-friendly normalization

`RequestID` must be treated as optional in Phase 3c.

Reason:

- the engine model already has `OwnershipMeta.RequestID`
- the current root SDK path does not yet expose a first-class request-id call option
- events emitted through the stable root SDK path may therefore leave `RequestID` empty unless a future additive request-metadata option is introduced

Recommended dispatcher responsibilities:

- async fan-out to sinks
- optional batch export to exporters
- bounded buffering
- explicit drop policy
- internal health counters such as dropped events and exporter failures
- bounded best-effort drain on `Shutdown(ctx)`

`Dispatcher.Shutdown(ctx)` should:

- stop accepting new events
- attempt to drain any buffered events within the caller's deadline
- flush batch exporters best-effort within that same deadline
- never extend or reinterpret the caller's shutdown deadline

Exporter implementations may optionally implement their own shutdown hook, but the dispatcher must own the final bounded drain contract seen by the root SDK.

### `inspect`

This package owns state materialization and queries.

Recommended public shapes:

```go
package inspect

type Store interface {
    Consume(context.Context, observe.Event) error
    Snapshot() Snapshot
    Events(Query) []observe.Event
    Subscribe(SubscriptionOptions) (Subscription, error)
}

type Snapshot struct {
    RuntimeLocks   []ActiveLock
    WorkerClaims   []ActiveClaim
    Renewals       []ActiveRenewal
    Clients        []ClientState
    Shutdown       ShutdownState
    Pipeline       PipelineState
    RecentEvents   []observe.Event
    GeneratedAt    time.Time
}
```

The default implementation should be an in-memory materialized store.

Recommended default:

- `inspect.NewStore(...)`
- store implements the event-consumer surface directly
- store receives normalized events and updates state online

This keeps the package usable both as:

- a direct query surface
- a direct bridge target for the root SDK path
- an optional sink in custom standalone wiring

`Store.Consume(...)` must be documented as a hot-path state-application contract, not as a generic extension point.

Rules for `Store.Consume(...)`:

- in-memory bookkeeping only
- no remote I/O
- no durable export
- no blocking subscriber fan-out
- errors are reserved for rejected local state application, not downstream delivery failures

Any slower subscription fan-out, SSE delivery, or richer query indexing should happen off that hot path.

### HTTP Surface

Phase 3c should provide default `net/http` handlers in `inspect`.

Recommended endpoints:

- `GET /locks/inspect`
- `GET /locks/inspect/active`
- `GET /locks/inspect/events`
- `GET /locks/inspect/stream`
- `GET /locks/inspect/health`

Semantics:

- `/locks/inspect`: full summary snapshot
- `/locks/inspect/active`: active runtime locks, worker claims, and renewals
- `/locks/inspect/events`: recent events with query filters
- `/locks/inspect/stream`: best-effort SSE stream of lifecycle events
- `/locks/inspect/health`: observer pipeline counters such as drops and exporter failures

Phase 3c should ship package APIs and HTTP handlers together. The handlers should remain thin wrappers over `inspect.Store`.

The HTTP docs must state that these endpoints expose the local process view only.

## Event Model

### Event Kinds

Phase 3c should introduce explicit event kinds rather than inferring meaning from counters.

Minimum event kinds:

- `acquire_started`
- `acquire_succeeded`
- `acquire_failed`
- `contention_detected`
- `overlap_rejected`
- `lease_renewed`
- `lease_lost`
- `released`
- `presence_checked`
- `idempotency_began`
- `idempotency_completed`
- `idempotency_failed`
- `shutdown_started`
- `shutdown_completed`

Optional phase-local refinement is acceptable if it does not fragment the model.

Examples:

- `claim_callback_started`
- `claim_callback_completed`
- `inspect_event_dropped`

But Phase 3c should avoid turning the event taxonomy into a giant engine-internal log vocabulary.

### Event Source Coverage

Events must cover all three execution surfaces:

- runtime manager
- worker manager
- client lifecycle

That means Phase 3c should include:

- `Run` and composite runtime events
- `Claim` and composite claim events
- renewal start and renewal failure facts
- idempotency begin and terminal state facts
- client shutdown and manager shutdown facts

Phase 3c must also define how those events reach both public layers that already exist in this repository:

- root SDK callers through `lockman.WithObserver(...)`, `lockman.WithInspectStore(...)`, or `lockman.WithObservability(...)`
- direct engine callers through an additive lower-level compatibility wiring path

Root SDK remains the preferred adoption path, but runtime and worker observability must not become root-only if the phase claims platform-wide lifecycle visibility.

### Normalization Rules

Rules:

- one event shape for both sync and async paths
- absent metadata remains empty rather than encoded through mode-specific types
- event fields should reflect execution facts, not inferred business meaning
- errors should be normalized as stable strings or error codes suitable for export

## Inspection State Model

### Materialized State

The inspection store should materialize at least:

- active runtime locks
- active worker claims
- active renewals
- current shutdown state for runtime, worker, and client paths
- dispatcher health counters
- recent event history

This state should be updated online by consuming `observe.Event` values.

For the root SDK path, current-state updates should be driven directly by the internal bridge before async export fan-out.

Reason:

- active-state views such as runtime locks, worker claims, renewals, and shutdown flags should not depend on a lossy exporter queue
- otherwise `/locks/inspect/active` could drift immediately after dropped release or shutdown events

Phase 3c should therefore distinguish:

- local state application to `inspect.Store`
- async export through `observe.Dispatcher`

This is a semantic distinction, not only an implementation detail.

The store is the local materialized-state component.
The dispatcher is the async export component.
Neither should be implemented in a way that weakens the other's guarantees.

### Recent Event History

Phase 3c should keep a bounded recent event history in memory.

Recommended behavior:

- ring-buffer style storage
- oldest entries dropped first
- queryable by lock id, resource key, owner id, event kind, and time window

This in-memory history is for near-real-time operator inspection. It does not replace durable export.

If history entries are dropped due to ring-buffer limits, current-state materialization must still remain correct for the local process.

### Streaming

The inspection store should support best-effort subscriptions for live debugging.

Recommended phase behavior:

- in-process subscription API
- HTTP SSE support through `/locks/inspect/stream`
- slow subscribers dropped or disconnected rather than blocking publishers

## Dispatcher And Export Design

### Async Fan-Out

The dispatcher should publish events asynchronously to:

- zero or more sinks
- zero or more batch exporters

Recommended data flow:

1. engine emits one normalized event
2. if an `inspect.Store` is configured through the root SDK path, the bridge applies the event to that store immediately
3. dispatcher enqueues a copy for async export
4. background workers fan out to sinks and exporters
5. sinks and exporters deliver logs, traces, audit records, or durable copies

### Durable Export Abstraction

Phase 3c should include exporter abstraction points so durable delivery can be added without changing runtime APIs.

However, first-party durable storage implementations are out of scope for this phase.

That means:

- exporter contracts belong in `observe`
- examples may show how to plug one in
- the repository does not need to ship Kafka, Postgres, or object-store exporters in Phase 3c

### Failure Behavior

Exporter and sink failures must be isolated.

Rules:

- sink failure increments health counters
- exporter failure increments health counters
- repeated failures may surface through inspection health and event stream
- no sink or exporter error is allowed to bubble into lock execution behavior
- dispatcher shutdown drain remains bounded by the provided context and may still drop late events when the deadline expires

## Wiring Into The Root SDK

Phase 3c should introduce root-level client wiring for observability.

Recommended option shapes:

```go
func WithObserver(dispatcher observe.Dispatcher) ClientOption
func WithInspectStore(store inspect.Store) ClientOption
```

Optional convenience wiring may also be added:

```go
type Observability struct {
    Dispatcher observe.Dispatcher
    Store      inspect.Store
}

func NewObservability(opts ...observe.Option) Observability
func WithObservability(obs Observability) ClientOption
```

The exact names may adjust, but the design goals are:

- root-level wiring for adopters
- no requirement that users manually instantiate engine managers
- no direct product dependency on `lockkit/observe.Recorder`

Required wiring semantics:

- `WithInspectStore(...)` must attach the store directly to the root SDK bridge for local state updates, even when no dispatcher is configured
- `WithObserver(...)` must attach async export only and must not be required for inspection snapshots to work
- `WithObservability(...)` should wire both together in the common case
- if both are configured independently, the bridge should write once to the store and publish once to the dispatcher rather than routing the store through the dispatcher by default

### Direct Engine Consumer Story

Phase 3c should keep the root SDK as the primary product surface, but it must also define an additive path for callers that construct engine managers directly.

Required compatibility direction:

- `lockkit/runtime` and `lockkit/workers` should each gain an additive observability wiring path
- that wiring path should feed the same normalized Phase 3c bridge model used by the root client
- repository-owned `examples/core` should be able to adopt Phase 3c observability without inventing private glue

Recommended shapes may vary, but the phase should land one explicit pattern such as:

- new manager constructors that accept observability config
- additive manager options for observability wiring
- a compatibility adapter package that bridges direct engine callers into the Phase 3c model

What Phase 3c should not do:

- leave direct runtime callers on `observe.NewNoopRecorder()` as the only realistic path
- leave direct worker callers without an equivalent lower-level observability story
- require repository examples to use bespoke test-only glue

### Migration Of The Existing Runtime Recorder

Phase 3c should treat `lockkit/observe.Recorder` as an internal compatibility seam, not as the final public surface.

Recommended migration:

- runtime and workers publish richer bridge events
- internal bridge can still satisfy or adapt existing recorder needs during migration
- new docs and examples should use root-level `observe` and `inspect`
- engine packages should not gain new public observability semantics through `lockkit/observe`
- the root client shutdown path should call `Dispatcher.Shutdown(ctx)` after publishing final shutdown events

## Tracing Design

### OTel-First Public Direction

Phase 3c tracing should be OTel-first.

Recommended spans from the base spec remain valid:

- `lock.execution`
- `lock.resolve`
- `lock.acquire`
- `lock.renew`
- `handler.execute`
- `guarded_write`
- `lock.release`

Phase 3c does not require that every engine package import OTel directly. It requires that the public `observe` package make OTel wiring the primary supported path.

### Audit Versus Trace

Tracing and audit must remain distinct concerns:

- tracing is for correlated execution spans and metrics
- audit is for event records that operators and downstream systems may retain or review

Phase 3c should use one shared event model but should not collapse tracing into raw audit log delivery.

## HTTP And Operational Guidance

Phase 3c should document that default inspection handlers are admin surfaces.

The package may ship plain `net/http` handlers, but production guidance must say:

- mount on an internal admin listener or protected route group
- apply service-specific authn and authz outside the library
- treat event streams as potentially sensitive operational data
- treat snapshot data as local-process telemetry rather than cluster-wide truth

Auth policy itself is out of scope for this phase.

## Testing Strategy

Phase 3c test coverage should include:

- event mapping tests for runtime, workers, and client lifecycle
- dispatcher tests for:
  - non-blocking publish
  - bounded buffering
  - drop behavior
  - sink isolation
  - exporter isolation
  - bounded shutdown drain and flush behavior
- inspection store tests for:
  - active lock materialization
  - worker claim materialization
  - renewal and lease-lost transitions
  - shutdown transitions
  - recent history truncation
  - event filtering
- HTTP tests for:
  - snapshot endpoint
  - active-state endpoint
  - event query endpoint
  - SSE streaming endpoint
  - health endpoint
- integration tests for:
  - sync execution
  - async claim execution
  - renewal failure
  - shutdown behavior
  - end-to-end inspection state updates

## Documentation And Examples

Phase 3c docs should add:

- a minimal observability wiring example
- an OTel wiring example
- an admin HTTP mounting example
- production guidance for inspection endpoint exposure

README updates should explain that:

- root SDK observability now exists through `observe` and `inspect`
- `inspect` is a process-local operational view, not correctness enforcement
- observability is best-effort and intentionally non-blocking

## Compatibility And Rollout

Phase 3c should be additive.

Compatibility requirements:

- existing `Run`, `Claim`, `CheckPresence`, and `Shutdown` semantics remain unchanged
- existing runtime metric recorder behavior should continue during migration
- root-level observability wiring should be optional
- no observability package should be required to construct a working client

## Recommendation

Phase 3c should implement:

- root-level `observe` package
- root-level `inspect` package
- internal bridge from runtime, workers, and client lifecycle into the public event model
- async best-effort dispatcher with sink and exporter support
- in-memory inspection store with current-state materialization and recent history
- default admin HTTP handlers and SSE streaming
- OTel-first public integration path
- examples and docs that mount and export the new surfaces

Phase 3c should not implement:

- first-party durable event store adapters
- auth policy for admin handlers
- changes to worker ack or retry policy
- strict lineage-specific or strict composite-specific new semantics

This is the smallest phase that makes the platform operationally inspectable and exportable in a product-shaped way while keeping the lock lifecycle itself simple and stable.
