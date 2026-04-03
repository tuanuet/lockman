# Production Guide

This guide answers the questions application teams ask before shipping `lockman` to production.

## Start Here

Start with one lock definition, one registry, one client, and one attached execution surface. Add more only after that first path is running in staging.

## Choose Run Or Claim

- Use `Run` for direct request/response or synchronous orchestration.
- Use `Claim` for queue delivery, retries, or redelivery-aware work.

Quickstarts:

- [`quickstart-sync.md`](quickstart-sync.md) — `Run` walkthrough
- [`quickstart-async.md`](quickstart-async.md) — `Claim` walkthrough
- [`runtime-vs-workers.md`](runtime-vs-workers.md) — choosing between the two models
- [`../examples/sdk/shared-lock-definition`](../examples/sdk/shared-lock-definition) — the canonical first `v1.3.0` SDK example

## Minimum Production Wiring

- `Run` requires a backend such as `github.com/tuanuet/lockman/backend/redis`.
- `Claim` always requires a backend, and `Claim` can use idempotency wiring such as `github.com/tuanuet/lockman/idempotency/redis` when the use case declares `lockman.Idempotent()`.
- Register all use cases at startup and fail fast on capability mismatches.

Minimal wiring pattern:

```go
var OrderDef = lockman.DefineLock(...)
var Approve = lockman.DefineRunOn("order.approve", OrderDef)

reg := lockman.NewRegistry()
if err := reg.Register(orderlocks.Approve); err != nil {
    return err
}

client, err := lockman.New(
    lockman.WithRegistry(reg),
    lockman.WithIdentity(lockman.Identity{OwnerID: "orders-api"}),
    lockman.WithBackend(backendredis.New(redisClient, "")),
)
if err != nil {
    return err
}
defer client.Shutdown(ctx)
```

## Stay On The Default Path

Prefer the root SDK unless a concrete stale-writer or multi-resource requirement proves otherwise. The root path keeps the learning surface small and avoids pulling in advanced modules before you need them.

On that root SDK path, use `DefineLock` plus `DefineRunOn`, `DefineHoldOn`, or `DefineClaimOn` for new code. Shorthand constructors like `DefineRun`, `DefineHold`, and `DefineClaim` are deprecated, but remain fully functional in the current release line for compatibility.

Advanced packages such as `advanced/strict` and `advanced/composite` are specialized surfaces and are outside the scope of this root-SDK shorthand deprecation pass.

## When Strict Is Worth It

Use `github.com/tuanuet/lockman/advanced/strict` when:

- a synchronous critical section needs fencing tokens for stale-writer protection
- you are doing compare-and-swap or guarded persistence
- downstream integrations must observe monotonic fencing tokens

Do not reach for strict just because "it feels safer." If you do not check `lease.FencingToken`, strict adds complexity with no benefit.

Details: [`advanced/strict.md`](advanced/strict.md)

## When Composite Is Worth It

Use `github.com/tuanuet/lockman/advanced/composite` when:

- one synchronous operation must hold multiple resources together (e.g. account transfers)
- you need two-resource consistency boundaries
- one callback should start only after multiple resources are acquired

If you only need a parent-child relationship on a single aggregate, a plain `Run` with shared resource IDs is usually enough. See [`examples/core/parent-lock-over-composite`](../examples/core/parent-lock-over-composite).

Details: [`advanced/composite.md`](advanced/composite.md)

## TTL And Renewal Mindset

- Default TTLs are tuned for typical request latencies. Shorten them only if you have measured contention.
- Long-running work inside a lock should use lease renewal, not oversized TTLs.
- If a callback takes longer than your TTL, the lease can expire before the work finishes — restructure the work instead of raising TTL.

## Identity And Ownership

- Set `OwnerID` to a stable identifier for your service or worker pool.
- The identity is not authentication. It is a label that makes lock ownership visible in observability and logs.
- Use one identity per logical deployment (e.g. "orders-api", "orders-worker"). Do not share identities across unrelated services.

## Production Checklist

1. All use cases are registered at startup.
2. `lockman.New(...)` succeeds before accepting traffic — fail fast on connection or capability errors.
3. `client.Shutdown(ctx)` is deferred so leases are released on process exit.
4. Backend connection strings come from configuration, not hardcoded values.
5. `Claim` paths have idempotency wiring and deliver `Delivery` metadata.
6. TTLs match your actual callback durations with headroom.
7. Logs include resource IDs and owner IDs for debugging contention.
8. Observability wiring is configured for monitoring and inspection.

## Observability And Inspection

`lockman` provides optional observability and inspection through the `observe` and `inspect` packages.

### Wiring Observability

```go
import (
    "github.com/tuanuet/lockman"
    "github.com/tuanuet/lockman/inspect"
    "github.com/tuanuet/lockman/observe"
)

// Create a dispatcher for async event export.
dispatcher := observe.NewDispatcher()
defer dispatcher.Shutdown(ctx)

// Create an inspect store for process-local state.
store := inspect.NewStore()

// Wire both via the convenience option.
client, err := lockman.New(
    lockman.WithRegistry(reg),
    lockman.WithIdentity(lockman.Identity{OwnerID: "orders-api"}),
    lockman.WithBackend(backend),
    lockman.WithObservability(lockman.Observability{
        Dispatcher: dispatcher,
        Store:      store,
    }),
)
```

### Mounting Inspection Endpoints

The inspect store provides HTTP handlers for admin inspection:

```go
mux := http.NewServeMux()
mux.Handle("/locks/", inspect.NewHandler(store))
```

**Important:** Inspection data is process-local telemetry, not cluster truth. It reflects the state of the current process only.

### Export Failure Semantics

Observability export failures do not fail the lock lifecycle. The `observe.Dispatcher` operates on a best-effort basis. If a sink or exporter fails, the lock acquisition or release continues normally.

## Common Mistakes

- Defining use cases inline at call time instead of at package scope.
- Treating shorthand constructors as recommended new-code APIs instead of deprecated compatibility helpers.
- Forgetting `lockman.Idempotent()` claim use cases require idempotency wiring — client startup will fail before the worker begins serving traffic.
- Using `Run` for queue consumers that receive retries or redeliveries.
- Raising TTL instead of restructuring slow callbacks.
- Sharing a single registry across unrelated boundaries — use separate registries for separate concerns.

## Which Example To Copy

| Scenario | Start from |
|---|---|
| Definition-first shared boundary | [`examples/sdk/shared-lock-definition`](../examples/sdk/shared-lock-definition) |
| Sync request/response lock | [`examples/sdk/sync-approve-order`](../examples/sdk/sync-approve-order) |
| Async queue consumer with idempotency | [`examples/sdk/async-process-order`](../examples/sdk/async-process-order) |
| Multi-resource transfer | [`examples/sdk/sync-transfer-funds`](../examples/sdk/sync-transfer-funds) |
| Strict fenced write | [`examples/sdk/sync-fenced-write`](../examples/sdk/sync-fenced-write) |
| Shared aggregate across sync and async flows | [`examples/sdk/shared-aggregate-split-definitions`](../examples/sdk/shared-aggregate-split-definitions) |
| Redis backend adapter example | [`backend/redis/examples/sync-approve-order`](../backend/redis/examples/sync-approve-order) |
| Redis idempotency adapter example | [`idempotency/redis/examples/async-process-order`](../idempotency/redis/examples/async-process-order) |
