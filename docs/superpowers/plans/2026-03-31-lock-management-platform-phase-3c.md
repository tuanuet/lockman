# Lock Management Platform Phase 3c Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Phase 3c observability and inspection so `lockman` can emit normalized lifecycle events, maintain process-local operational state, expose admin inspection endpoints, and wire both root SDK callers and direct engine callers into the same model without changing lock semantics.

**Architecture:** Build Phase 3c in four layers. First add root-level `observe` contracts and a bounded async dispatcher. Then add root-level `inspect` with an in-memory hot-path state store, recent history, subscriptions, and HTTP handlers. Next add an internal bridge plus additive root and engine wiring so runtime, workers, and client shutdown emit one normalized event model. Finish with docs, examples, and full verification. Keep `inspect` state application synchronous and in-memory, while all export delivery remains async and best-effort.

**Tech Stack:** Go 1.22+, standard library `context`, `net/http`, `testing`, `httptest`, `encoding/json`, `sync`, `sync/atomic`, existing `lockman` root client, existing `lockkit/runtime`, existing `lockkit/workers`, existing `lockkit/observe.Recorder`, optional OpenTelemetry adapter surface at the root package boundary

---

## Planned File Structure

### Existing files to modify

- `go.mod`: add any root-module dependencies needed by the chosen narrow OTel adapter surface
- `go.sum`: record checksums for any new root-module observability dependencies
- `client.go`: add root client observability fields, bridge creation, manager wiring, and shutdown drain sequencing
- `identity.go`: extend `clientConfig` and add root observability client options
- `client_test.go`: cover startup wiring, optional observability config, and shutdown sequencing
- `client_run_test.go`: verify root SDK `Run` emits observability state/events through configured store and dispatcher
- `client_claim_test.go`: verify root SDK `Claim` emits worker/renewal/idempotency observability state/events
- `README.md`: add Phase 3c observability overview, wiring snippet, and inspect/admin references
- `docs/production-guide.md`: add admin mounting and process-local inspection guidance
- `docs/runtime-vs-workers.md`: explain observability applies to both run and claim paths
- `docs/errors.md`: clarify observability/export failures are not surfaced as lock lifecycle errors
- `adoption_surface_test.go`: pin README and docs references for the new observe/inspect adoption surface
- `examples/README.md`: point readers to the new observability example(s)
- `lockkit/runtime/manager.go`: add additive variadic observability options and bridge field while preserving existing recorder arg compatibility
- `lockkit/runtime/exclusive.go`: emit normalized runtime acquire/release/contention/overlap events through the bridge
- `lockkit/runtime/composite.go`: emit composite acquire/release events and active-state updates through the bridge
- `lockkit/runtime/presence.go`: emit presence-check observability events
- `lockkit/runtime/presence_test.go`: add focused presence-check event coverage
- `lockkit/runtime/shutdown_test.go`: extend shutdown coverage for observability state/events
- `lockkit/runtime/exclusive_test.go`: add focused runtime event/state tests
- `lockkit/runtime/composite_test.go`: add focused composite runtime event/state tests
- `lockkit/workers/manager.go`: add additive variadic observability options and bridge field
- `lockkit/workers/execute.go`: emit claim, idempotency, renewal, lease-lost, and release observability events
- `lockkit/workers/execute_composite.go`: emit composite worker claim and renewal events
- `lockkit/workers/renewal.go`: emit renew-success and renew-failure facts through the bridge
- `lockkit/workers/shutdown.go`: emit shutdown lifecycle events and integrate with bounded dispatcher drain expectations
- `lockkit/workers/manager_test.go`: add worker observability constructor and shutdown coverage
- `lockkit/workers/execute_test.go`: add focused worker event/state tests
- `lockkit/workers/execute_composite_test.go`: add focused composite worker event/state tests
- `lockkit/observe/contracts.go`: keep or reduce to legacy runtime metric compatibility shim during migration

### New root-level public packages and files

- `observe/event.go`: public event kinds, event struct, and stable normalization types
- `observe/dispatcher.go`: bounded async dispatcher implementation, publish path, bounded `Shutdown(ctx)`, and sink/exporter orchestration
- `observe/options.go`: dispatcher options such as buffer size, drop policy, sinks, exporters, and health tuning
- `observe/noop.go`: no-op sink/exporter helpers and minimal defaults
- `observe/dispatcher_test.go`: bounded buffering, drop, shutdown drain, sink isolation, and exporter isolation tests
- `observe/event_test.go`: event-kind and field-stability tests
- `observe/otel.go` or `observe/otel_adapter.go`: OTel-first root adapter surface
- `observe/otel_test.go`: focused adapter compile-time and minimal behavior tests
- `inspect/types.go`: snapshot, active-state, query, subscription, and pipeline-health types
- `inspect/store.go`: in-memory hot-path store implementing `Consume(...)`, snapshot queries, and recent-history maintenance
- `inspect/store_test.go`: state materialization, ring-buffer truncation, subscription isolation, and query filtering tests
- `inspect/http.go`: default HTTP handlers for snapshot, active state, events, health, and SSE stream
- `inspect/http_test.go`: `httptest` coverage for endpoint shapes and filtering

### New root-internal bridge files

- `internal/observebridge/bridge.go`: bridge contract shared by client, runtime, and workers
- `internal/observebridge/event_builder.go`: helpers mapping engine lifecycle facts into `observe.Event`
- `internal/observebridge/runtime.go`: runtime-specific event construction helpers
- `internal/observebridge/workers.go`: worker-specific event construction helpers
- `internal/observebridge/client.go`: root-client startup and shutdown event helpers
- `internal/observebridge/options.go`: bridge wiring helpers that combine direct store application plus async dispatcher export
- `internal/observebridge/bridge_test.go`: exact field-mapping and once-only publish/store tests

### New or updated examples

- `examples/sdk/observability-basic/main.go`: root-SDK example wiring `WithObservability(...)`, mounting `inspect` HTTP handlers, and printing one or two lifecycle facts
- `examples/sdk/observability-basic/main_test.go`: deterministic output contract test
- `examples/sdk/observability-basic/README.md`: how to run and what endpoints to call
- `examples/core/observability-runtime/main.go`: direct `runtime.NewManager(..., opts...)` example proving lower-level compatibility
- `examples/core/observability-runtime/main_test.go`: deterministic lower-level compatibility test
- `examples/core/observability-runtime/README.md`: how to run the lower-level manager example

## Phase Scope

This plan delivers only what the Phase 3c design requires:

- root-level `observe` and `inspect` packages
- a bounded async dispatcher with best-effort export semantics
- a process-local in-memory inspection store with hot-path state updates
- admin HTTP handlers and SSE streaming
- one normalized lifecycle event model covering runtime, workers, and client shutdown
- additive root client wiring and additive direct engine compatibility wiring
- OTel-first adapter surface at the root package boundary
- docs and examples showing root and lower-level adoption

It does **not** implement:

- first-party durable event-store adapters
- authn/authz for admin endpoints
- changes to lock behavior, retry policy, or ack policy
- strict lineage-specific or strict composite-specific new semantics
- cluster-wide inspection aggregation

## Implementation Notes

- Use @superpowers:test-driven-development for every task. Write the failing test first, run it, then make the smallest implementation change that passes.
- Use @superpowers:verification-before-completion before claiming the phase is done.
- Use @superpowers:requesting-code-review after implementation tasks are complete and before merge.
- Keep `inspect.Store.Consume(...)` as hot-path in-memory bookkeeping only. Do not let logging, tracing export, SSE fan-out, or durable export run on that path.
- Keep `observe.Dispatcher.Publish(...)` non-blocking. Publish must never wait for exporters or slow subscribers.
- Root SDK wiring should apply local state to `inspect.Store` directly and publish a copy to the async dispatcher. Do not route the store through the dispatcher by default.
- Choose additive variadic observability options for `runtime.NewManager(...)` and `workers.NewManager(...)` rather than creating replacement constructors. Existing callsites must continue to compile unchanged.
- Preserve `lockkit/observe.Recorder` behavior during migration, but do not expand it into the new public surface. Use it only as a compatibility seam while the engine bridge lands.
- Make `inspect` docs and HTTP handlers explicit that the snapshot is process-local telemetry, not correctness enforcement or cluster truth.
- `Dispatcher.Shutdown(ctx)` must use the caller's deadline as-is. No hidden retries, sleeps, or extended draining beyond `ctx`.
- `shutdown_completed` is best-effort. If the deadline expires first, health counters should show the drop rather than blocking shutdown longer.
- Keep the initial OTel integration narrow: root-level adapter constructor(s), no direct OTel imports in engine packages.

### Task 1: Add The Root `observe` Event Model And Bounded Dispatcher

**Files:**
- Create: `observe/event.go`
- Create: `observe/dispatcher.go`
- Create: `observe/options.go`
- Create: `observe/noop.go`
- Create: `observe/event_test.go`
- Create: `observe/dispatcher_test.go`

- [ ] **Step 1: Write the failing event and dispatcher tests**

Add tests that pin:

- stable event-kind strings for the required Phase 3c kinds
- `Publish(...)` returns immediately even when a sink blocks
- bounded buffer drop behavior increments internal health counters
- `Shutdown(ctx)` drains queued events best-effort without exceeding the caller deadline
- sink and exporter failures do not stop delivery to the remaining sinks/exporters

Example dispatcher test scaffold:

```go
func TestDispatcherPublishDoesNotBlockOnSlowSink(t *testing.T) {
	slow := make(chan struct{})
	d := observe.NewDispatcher(
		observe.WithBufferSize(1),
		observe.WithSink(observe.SinkFunc(func(context.Context, observe.Event) error {
			<-slow
			return nil
		})),
	)
	defer func() { _ = d.Shutdown(context.Background()) }()

	done := make(chan struct{})
	go func() {
		d.Publish(observe.Event{Kind: observe.EventAcquireStarted})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("Publish blocked on slow sink")
	}
	close(slow)
}
```

- [ ] **Step 2: Run the focused observe package tests to verify they fail**

Run: `go test ./observe -run 'Dispatcher|Event' -v`
Expected: FAIL because the `observe` package does not exist yet.

- [ ] **Step 3: Implement `observe/event.go` and `observe/options.go`**

Add:

- `EventKind` constants for the required lifecycle events
- `Event` with normalized lifecycle metadata and optional `RequestID`
- sink/exporter function adapters if useful, such as:

```go
type SinkFunc func(context.Context, Event) error

func (f SinkFunc) Consume(ctx context.Context, event Event) error { return f(ctx, event) }
```

- dispatcher options for buffer size, drop policy, sinks, exporters, and worker count

- [ ] **Step 4: Implement the bounded async dispatcher**

In `observe/dispatcher.go`, implement:

- a buffered publish queue
- non-blocking `Publish(...)`
- worker goroutines for sink/exporter fan-out
- health counters for dropped events, sink failures, exporter failures
- bounded `Shutdown(ctx)` drain semantics

Keep the dispatcher root-facing and product-facing. Do not import `lockkit` packages here.

- [ ] **Step 5: Run the observe package tests to verify they pass**

Run: `go test ./observe -run 'Dispatcher|Event' -v`
Expected: PASS

- [ ] **Step 6: Commit the observe dispatcher batch**

```bash
git add observe/event.go observe/options.go observe/dispatcher.go observe/noop.go observe/event_test.go observe/dispatcher_test.go
git commit -m "feat: add observe event model and dispatcher"
```

### Task 2: Add The Root `inspect` Hot-Path Store, Queries, And HTTP Handlers

**Files:**
- Create: `inspect/types.go`
- Create: `inspect/store.go`
- Create: `inspect/http.go`
- Create: `inspect/store_test.go`
- Create: `inspect/http_test.go`

- [ ] **Step 1: Write the failing inspect store and HTTP tests**

Add tests that pin:

- `Store.Consume(...)` materializes runtime locks, worker claims, renewals, and shutdown state correctly
- recent event history truncates with oldest-first drop while current state stays correct
- query filters by `lock_id`, `resource_key`, `owner_id`, `kind`, and time window
- slow subscribers do not block `Consume(...)`
- HTTP endpoints return the expected JSON or SSE shapes

Example state test scaffold:

```go
func TestStoreConsumeAppliesRuntimeReleaseWithoutBreakingHistory(t *testing.T) {
	store := inspect.NewStore(inspect.WithHistoryLimit(2))

	_ = store.Consume(context.Background(), observe.Event{
		Kind:        observe.EventAcquireSucceeded,
		LockID:      "order.approve",
		ResourceKey: "order:123",
		OwnerID:     "orders-api",
	})
	_ = store.Consume(context.Background(), observe.Event{
		Kind:        observe.EventReleased,
		LockID:      "order.approve",
		ResourceKey: "order:123",
		OwnerID:     "orders-api",
	})

	snap := store.Snapshot()
	if len(snap.RuntimeLocks) != 0 {
		t.Fatalf("expected no active runtime locks, got %#v", snap.RuntimeLocks)
	}
}
```

- [ ] **Step 2: Run the focused inspect package tests to verify they fail**

Run: `go test ./inspect -run 'Store|HTTP' -v`
Expected: FAIL because the `inspect` package does not exist yet.

- [ ] **Step 3: Implement `inspect/types.go` and `inspect/store.go`**

Create the in-memory store with:

- `Snapshot`
- `PipelineState`
- active runtime lock / worker claim / renewal records
- recent event history ring buffer
- subscription registration and drop-on-slow-delivery behavior

`Store.Consume(...)` must stay in-memory only. Push subscriber fan-out onto non-blocking channels or background goroutines so the hot path remains narrow.

- [ ] **Step 4: Implement the default HTTP handlers**

In `inspect/http.go`, add:

- `NewHandler(store Store, opts ...HandlerOption) http.Handler`
- route handling for:
  - `GET /locks/inspect`
  - `GET /locks/inspect/active`
  - `GET /locks/inspect/events`
  - `GET /locks/inspect/health`
  - `GET /locks/inspect/stream`

Keep handlers thin. They should call store query methods and encode results. Do not let handlers read engine internals directly.

- [ ] **Step 5: Run the inspect package tests to verify they pass**

Run: `go test ./inspect -run 'Store|HTTP' -v`
Expected: PASS

- [ ] **Step 6: Commit the inspect package batch**

```bash
git add inspect/types.go inspect/store.go inspect/http.go inspect/store_test.go inspect/http_test.go
git commit -m "feat: add inspect store and admin handlers"
```

### Task 3: Add The Internal Bridge And Root Client Observability Scaffolding

**Files:**
- Create: `internal/observebridge/bridge.go`
- Create: `internal/observebridge/event_builder.go`
- Create: `internal/observebridge/runtime.go`
- Create: `internal/observebridge/workers.go`
- Create: `internal/observebridge/client.go`
- Create: `internal/observebridge/options.go`
- Create: `internal/observebridge/bridge_test.go`
- Modify: `identity.go`
- Modify: `client.go`
- Modify: `client_test.go`
- Modify: `client_run_test.go`
- Modify: `client_claim_test.go`

- [ ] **Step 1: Write the failing bridge and root-client tests**

Add tests that pin:

- `WithInspectStore(...)`, `WithObserver(...)`, and `WithObservability(...)` populate root config correctly
- bridge local-state and async-export semantics stay once-only
- `RequestID` remains optional on the root path
- bridge-level shutdown helper uses the dispatcher shutdown contract without extending the caller deadline

Example bridge test scaffold:

```go
func TestBridgeWritesOnceToStoreAndOnceToDispatcher(t *testing.T) {
	var storeCalls int
	var publishCalls int

	store := &stubStore{
		consume: func(context.Context, observe.Event) error {
			storeCalls++
			return nil
		},
	}
	dispatcher := &stubDispatcher{
		publish: func(observe.Event) {
			publishCalls++
		},
	}

	bridge := observebridge.New(observebridge.Config{
		Store:      store,
		Dispatcher: dispatcher,
	})
	bridge.PublishRuntimeAcquireSucceeded(...)

	if storeCalls != 1 || publishCalls != 1 {
		t.Fatalf("unexpected calls store=%d publish=%d", storeCalls, publishCalls)
	}
}
```

- [ ] **Step 2: Run the focused bridge and root-client tests to verify they fail**

Run: `go test ./internal/observebridge ./ -run 'Bridge|Observability|Client' -v`
Expected: FAIL because the bridge and root observability options do not exist yet.

- [ ] **Step 3: Implement the internal bridge**

In `internal/observebridge`, add:

- a small bridge type that accepts an `inspect.Store` and `observe.Dispatcher`
- helpers for publishing root/client/runtime/worker lifecycle events
- strict once-only semantics so the bridge applies state locally and publishes one async copy

Keep the bridge root-internal. It should depend on `observe`, `inspect`, and engine definition types, but not become a public package.

- [ ] **Step 4: Implement root client options and wiring**

In `identity.go` and `client.go`, add:

- `WithObserver(dispatcher observe.Dispatcher) ClientOption`
- `WithInspectStore(store inspect.Store) ClientOption`
- `WithObservability(obs Observability) ClientOption`
- `Observability` bundle type if needed

Stage the root client so:

- observability config is captured once during `New(...)`
- the bridge can be created once during `New(...)`
- final manager handoff is deferred until Tasks 4 and 5 land the additive manager options
- final shutdown sequencing is completed in the dedicated root-integration task after engine wiring exists

- [ ] **Step 5: Run the focused bridge and root-client tests to verify they pass**

Run: `go test ./internal/observebridge ./ -run 'Bridge|Observability|ClientOption' -v`
Expected: PASS

- [ ] **Step 6: Commit the bridge and root wiring batch**

```bash
git add internal/observebridge identity.go client.go client_test.go client_run_test.go client_claim_test.go
git commit -m "feat: add observability bridge and root config scaffolding"
```

### Task 4: Add Additive Runtime Manager Observability Wiring

**Files:**
- Modify: `lockkit/runtime/manager.go`
- Modify: `lockkit/runtime/exclusive.go`
- Modify: `lockkit/runtime/composite.go`
- Modify: `lockkit/runtime/presence.go`
- Modify: `lockkit/runtime/exclusive_test.go`
- Modify: `lockkit/runtime/composite_test.go`
- Modify: `lockkit/runtime/presence_test.go`
- Modify: `lockkit/runtime/shutdown_test.go`
- Modify: `lockkit/observe/contracts.go`

- [ ] **Step 1: Write the failing runtime observability tests**

Add tests that pin:

- `runtime.NewManager(...)` still compiles with the existing `(reg, driver, recorder)` shape
- additive variadic runtime options can attach the Phase 3c bridge
- `ExecuteExclusive` emits acquire, contention, overlap, release, and active-state changes
- `ExecuteCompositeExclusive` emits composite acquire/release facts without double-counting active state
- `CheckPresence(...)` emits presence-check events
- shutdown emits runtime shutdown lifecycle facts

Example constructor expectation:

```go
func TestNewManagerAcceptsOptionalObservabilityOptions(t *testing.T) {
	_, err := runtime.NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder(), runtime.WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
}
```

- [ ] **Step 2: Run the focused runtime tests to verify they fail**

Run: `go test ./lockkit/runtime -run 'Observability|Presence|Shutdown|Composite|ExecuteExclusive' -v`
Expected: FAIL because runtime observability options and event emission do not exist yet.

- [ ] **Step 3: Add additive runtime observability options**

Update `runtime.NewManager(...)` to:

- keep the existing three parameters
- accept variadic runtime options after the recorder
- preserve existing callers unchanged

Recommended shape:

```go
func NewManager(reg registry.Reader, driver backend.Driver, recorder observe.Recorder, opts ...Option) (*Manager, error)
```

with an option such as:

```go
func WithBridge(b Bridge) Option
```

- [ ] **Step 4: Emit runtime lifecycle events through the bridge**

Update runtime execution and presence paths so the bridge receives:

- acquire start/success/failure
- contention and overlap rejection
- active-state changes
- release
- presence checks
- shutdown start/completion

Keep `lockkit/observe.Recorder` working during migration, but do not expand it. Let it coexist with the new bridge until the phase is complete.

- [ ] **Step 5: Run the focused runtime tests to verify they pass**

Run: `go test ./lockkit/runtime -run 'Observability|Presence|Shutdown|Composite|ExecuteExclusive' -v`
Expected: PASS

- [ ] **Step 6: Commit the runtime observability batch**

```bash
git add lockkit/runtime/manager.go lockkit/runtime/exclusive.go lockkit/runtime/composite.go lockkit/runtime/presence.go lockkit/runtime/exclusive_test.go lockkit/runtime/composite_test.go lockkit/runtime/presence_test.go lockkit/runtime/shutdown_test.go lockkit/observe/contracts.go
git commit -m "feat: add runtime observability bridge wiring"
```

### Task 5: Add Additive Worker Manager Observability Wiring

**Files:**
- Modify: `lockkit/workers/manager.go`
- Modify: `lockkit/workers/execute.go`
- Modify: `lockkit/workers/execute_composite.go`
- Modify: `lockkit/workers/renewal.go`
- Modify: `lockkit/workers/shutdown.go`
- Modify: `lockkit/workers/manager_test.go`
- Modify: `lockkit/workers/execute_test.go`
- Modify: `lockkit/workers/execute_composite_test.go`

- [ ] **Step 1: Write the failing worker observability tests**

Add tests that pin:

- `workers.NewManager(...)` still compiles with the existing `(reg, driver, store)` shape
- additive variadic worker options can attach the Phase 3c bridge
- `ExecuteClaimed` emits claim lifecycle, idempotency lifecycle, renewal success, lease loss, and release facts
- worker shutdown emits start/completion facts
- composite claims emit member-safe lifecycle facts without double-publishing the same logical event

Example renewal expectation:

```go
func TestExecuteClaimedEmitsLeaseLostWhenRenewalFails(t *testing.T) {
	var events []observe.Event
	bridge := workerTestBridge(func(event observe.Event) {
		events = append(events, event)
	})
	mgr, err := workers.NewManager(reg, driver, store, workers.WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteClaimed(context.Background(), req, func(ctx context.Context, claim definitions.ClaimContext) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !hasEventKind(events, observe.EventLeaseLost) {
		t.Fatal("expected lease_lost event")
	}
}
```

- [ ] **Step 2: Run the focused worker tests to verify they fail**

Run: `go test ./lockkit/workers -run 'Observability|ExecuteClaimed|Shutdown|Composite|Strict' -v`
Expected: FAIL because worker observability options and event emission do not exist yet.

- [ ] **Step 3: Add additive worker observability options**

Update `workers.NewManager(...)` to:

- keep the existing three parameters
- accept variadic worker options after the store
- preserve existing callers unchanged

Recommended shape:

```go
func NewManager(reg registry.Reader, driver backend.Driver, store idempotency.Store, opts ...Option) (*Manager, error)
```

- [ ] **Step 4: Emit worker lifecycle, idempotency, and renewal events**

Update worker execution paths so the bridge receives:

- acquire start/success/failure
- idempotency begin/completed/failed facts
- renewal success and lease-lost facts
- release
- shutdown start/completion

Keep publish semantics aligned with the root SDK bridge model and avoid double-writing to local state.

- [ ] **Step 5: Run the focused worker tests to verify they pass**

Run: `go test ./lockkit/workers -run 'Observability|ExecuteClaimed|Shutdown|Composite|Strict' -v`
Expected: PASS

- [ ] **Step 6: Commit the worker observability batch**

```bash
git add lockkit/workers/manager.go lockkit/workers/execute.go lockkit/workers/execute_composite.go lockkit/workers/renewal.go lockkit/workers/shutdown.go lockkit/workers/manager_test.go lockkit/workers/execute_test.go lockkit/workers/execute_composite_test.go
git commit -m "feat: add worker observability bridge wiring"
```

### Task 6: Complete Root Client Integration And Shutdown Sequencing

**Files:**
- Modify: `client.go`
- Modify: `client_test.go`
- Modify: `client_run_test.go`
- Modify: `client_claim_test.go`

- [ ] **Step 1: Write the failing root integration tests**

Add tests that pin:

- `WithInspectStore(...)` updates local state even when no dispatcher is configured
- `WithObserver(...)` publishes async export without requiring an inspect store
- `WithObservability(...)` wires both once, without double-writing to the store
- root `Run` and `Claim` paths surface normalized event fields once runtime and worker manager options exist
- client shutdown publishes final shutdown facts and then calls `Dispatcher.Shutdown(ctx)`

- [ ] **Step 2: Run the focused root integration tests to verify they fail**

Run: `go test ./ -run 'Observability|Client|Run|Claim|Shutdown' -v`
Expected: FAIL because root client integration is not complete yet.

- [ ] **Step 3: Finish root client manager wiring**

Update `client.go` so:

- `New(...)` creates one bridge from configured store/dispatcher
- runtime manager receives the bridge through the additive runtime option
- worker manager receives the bridge through the additive worker option
- existing clients without observability config still build unchanged

- [ ] **Step 4: Finish root shutdown sequencing**

Update `client.Shutdown(ctx)` so:

- final client shutdown events are published before dispatcher drain
- `Dispatcher.Shutdown(ctx)` is called exactly once after manager shutdown paths return
- dispatcher drain remains bounded by the caller context

- [ ] **Step 5: Run the focused root integration tests to verify they pass**

Run: `go test ./ -run 'Observability|Client|Run|Claim|Shutdown' -v`
Expected: PASS

- [ ] **Step 6: Commit the root integration batch**

```bash
git add client.go client_test.go client_run_test.go client_claim_test.go
git commit -m "feat: wire root client observability integration"
```

### Task 7: Add The Root OTel Adapter Surface

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `observe/otel.go` or `observe/otel_adapter.go`
- Create: `observe/otel_test.go`

- [ ] **Step 1: Write the failing OTel adapter tests**

Add tests that pin:

- the root `observe` package exposes an OTel-first adapter constructor
- the adapter satisfies the `Sink` contract
- the adapter maps at least a small fixed set of event kinds into span or metric recording calls

Keep these tests narrow. Do not attempt a full OTel integration suite in this phase.

- [ ] **Step 2: Run the focused OTel adapter tests to verify they fail**

Run: `go test ./observe -run 'OTel|OpenTelemetry' -v`
Expected: FAIL because the adapter does not exist yet.

- [ ] **Step 3: Implement the narrow OTel adapter**

Provide a root-facing adapter constructor such as:

```go
func NewOTelSink(provider trace.TracerProvider, meter metric.Meter) Sink
```

or an equivalent small config type.

Keep this adapter:

- root-facing
- optional
- out of engine packages

If the chosen implementation needs new root-module dependencies, update `go.mod` and `go.sum` in this task and keep the dependency set as small as possible.

- [ ] **Step 4: Run the focused OTel adapter tests to verify they pass**

Run: `go test ./observe -run 'OTel|OpenTelemetry' -v`
Expected: PASS

- [ ] **Step 5: Commit the OTel adapter batch**

```bash
git add go.mod go.sum observe/otel.go observe/otel_test.go
git commit -m "feat: add observe otel adapter"
```

### Task 8: Add Docs, Examples, And Full-Phase Verification

**Files:**
- Modify: `README.md`
- Modify: `docs/production-guide.md`
- Modify: `docs/runtime-vs-workers.md`
- Modify: `docs/errors.md`
- Modify: `adoption_surface_test.go`
- Modify: `examples/README.md`
- Create: `examples/sdk/observability-basic/main.go`
- Create: `examples/sdk/observability-basic/main_test.go`
- Create: `examples/sdk/observability-basic/README.md`
- Create: `examples/core/observability-runtime/main.go`
- Create: `examples/core/observability-runtime/main_test.go`
- Create: `examples/core/observability-runtime/README.md`

- [ ] **Step 1: Write the failing example/output and docs-surface tests**

Add or update tests that pin:

- the new example output remains deterministic
- `adoption_surface_test.go` adds observability-specific assertions, using test names such as `TestObservabilityDocs...` and `TestInspectProcessLocal...`, for `observe`, `inspect`, and the new example references
- docs explicitly say inspection is process-local and admin-oriented

Example output test scaffold:

```go
func TestMainPrintsInspectEndpointAndLifecycleSummary(t *testing.T) {
	out := runMain(t)
	for _, want := range []string{
		"inspect endpoint: /locks/inspect",
		"recent events:",
		"active runtime locks: 0",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run the focused example and docs tests to verify they fail**

Run: `go test ./ -run 'ObservabilityDocs|InspectProcessLocal' -v`
Expected: FAIL because the new docs-surface assertions do not exist yet.

Run: `(cd examples && go test -tags lockman_examples ./sdk/observability-basic ./core/observability-runtime -v)`
Expected: FAIL because the new example packages do not exist yet.

Run: `rg -n "observe|inspect|process-local|admin" README.md docs/production-guide.md docs/runtime-vs-workers.md docs/errors.md`
Expected: missing or incomplete references.

Run: `rg -n "observe|inspect|runtime.NewManager|lockman_examples" examples/README.md examples/sdk/observability-basic/README.md examples/core/observability-runtime/README.md`
Expected: missing or incomplete example-guide references.

- [ ] **Step 3: Implement the docs and example updates**

Update docs so they explain:

- how to wire `WithObservability(...)`
- how to mount `inspect.NewHandler(...)`
- why snapshot data is process-local
- why export failure does not fail the lock lifecycle

Add both:

- a runnable root-SDK example proving the default adoption path
- a small direct-engine example proving the lower-level compatibility story from the design

- [ ] **Step 4: Run the full phase verification**

Run:

```bash
go test ./observe ./inspect ./internal/observebridge ./lockkit/runtime ./lockkit/workers ./... -v
```

Expected: PASS

Run:

```bash
(cd examples && go test -tags lockman_examples ./sdk/observability-basic ./core/observability-runtime -v)
```

Expected: PASS

Run:

```bash
rg -n "observe|inspect|process-local|admin" README.md docs/production-guide.md docs/runtime-vs-workers.md docs/errors.md
```

Expected: matching references in each updated doc.

Run:

```bash
rg -n "observe|inspect|runtime.NewManager|lockman_examples" examples/README.md examples/sdk/observability-basic/README.md examples/core/observability-runtime/README.md
```

Expected: matching references in the examples index and both new example READMEs.

- [ ] **Step 5: Commit the docs and verification batch**

```bash
git add README.md docs/production-guide.md docs/runtime-vs-workers.md docs/errors.md adoption_surface_test.go examples/README.md examples/sdk/observability-basic examples/core/observability-runtime
git commit -m "docs: add phase 3c observability guidance"
```

## Plan Review Checklist

Use this checklist while executing or reviewing the work:

- root `observe` package stays free of `lockkit` imports
- root `inspect` store hot path remains in-memory and non-blocking
- root client applies local state directly and exports async through the dispatcher
- `Dispatcher.Shutdown(ctx)` is bounded by the caller deadline
- `runtime.NewManager(...)` and `workers.NewManager(...)` remain additive and backward-compatible
- direct engine callers get a first-class compatibility path
- `lockkit/observe.Recorder` is not expanded into the new public API
- docs call `inspect` process-local and admin-oriented

## Review Loop Result

This plan was self-reviewed and revised in-loop against the approved Phase 3c design until no blocking plan issues remained. In particular, the plan now explicitly:

- chooses additive variadic manager options for lower-level engine observability wiring
- separates bridge/config scaffolding from final root client integration so task order is executable
- keeps `inspect.Store.Consume(...)` as a narrow hot-path contract
- separates direct local state application from async export delivery
- includes bounded dispatcher shutdown verification
