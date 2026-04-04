# lockman

`lockman` is a typed Go SDK for locking business use cases with one simple path for sync, hold, and async workflows.

From `v1.3.0`, the public SDK story is definition-first:

1. define one lock boundary
2. attach one or more execution surfaces to it
3. register centrally
4. call `Run`, `Hold`, or `Claim`
5. call `RunMultiple` or `HoldMultiple` for batch operations on the same definition

## Why lockman

- Bind typed input to a lock definition instead of building lock keys by hand at callsites.
- Sync, hold, and async flows share one business boundary instead of feeling like separate products.
- The happy path stays short, but stricter coordination features are available when you need them.

## Install

```bash
go get github.com/tuanuet/lockman
```

## Quick Start

```go
package orderlocks

import (
	"context"

	"github.com/tuanuet/lockman"
	backendredis "github.com/tuanuet/lockman/backend/redis"
)

type ApproveInput struct {
	OrderID string
}

var OrderDef = lockman.DefineLock(
	"order",
	lockman.BindResourceID("order", func(in ApproveInput) string { return in.OrderID }),
)

var Approve = lockman.DefineRunOn("order.approve", OrderDef)

func approve(ctx context.Context, redisClient any) error {
	reg := lockman.NewRegistry()
	if err := reg.Register(Approve); err != nil {
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

	req, err := Approve.With(ApproveInput{OrderID: "123"})
	if err != nil {
		return err
	}

	return client.Run(ctx, req, func(ctx context.Context, lease lockman.Lease) error {
		return approveOrder(ctx, "123")
	})
}
```

The smallest runnable version: [`examples/sdk/shared-lock-definition`](examples/sdk/shared-lock-definition).

## Run vs Hold vs Claim

| Surface | When to use | Example |
|---------|-------------|---------|
| `Run` | Synchronous critical sections (request/response, job orchestration) | [`examples/sdk/sync-approve-order`](examples/sdk/sync-approve-order) |
| `RunMultiple` | Batch sync operations on multiple keys of the same definition | [`examples/sdk/multiple-run`](examples/sdk/multiple-run) |
| `Hold` | Retain a manual lock across steps (approval windows, admin holds) | [`examples/sdk/manual-hold`](examples/sdk/manual-hold) |
| `HoldMultiple` | Batch hold on multiple keys of the same definition | [`examples/sdk/multiple-hold`](examples/sdk/multiple-hold) |
| `Claim` | Async delivery with idempotency (retry/redelivery dedup) | [`examples/sdk/async-process-order`](examples/sdk/async-process-order) |

## Examples

Start with `examples/sdk` (workspace mirrors of the public SDK interface):

- [`examples/sdk/shared-lock-definition`](examples/sdk/shared-lock-definition) – canonical first example
- [`examples/sdk/sync-approve-order`](examples/sdk/sync-approve-order) – shortest sync `Run` flow
- [`examples/sdk/manual-hold`](examples/sdk/manual-hold) – hold acquire/forfeit flow
- [`examples/sdk/async-process-order`](examples/sdk/async-process-order) – async `Claim` with idempotency
- [`examples/sdk/shared-aggregate-split-definitions`](examples/sdk/shared-aggregate-split-definitions) – sync + async over one aggregate
- [`examples/sdk/parent-lock-over-composite`](examples/sdk/parent-lock-over-composite) – parent lock vs composite
- [`examples/sdk/sync-transfer-funds`](examples/sdk/sync-transfer-funds) – multi-resource sync lock
- [`examples/sdk/sync-fenced-write`](examples/sdk/sync-fenced-write) – strict fenced execution
- [`examples/sdk/multiple-run`](examples/sdk/multiple-run) – batch multi-key same-definition acquire
- [`examples/sdk/multiple-hold`](examples/sdk/multiple-hold) – batch multi-key same-definition hold
- [`examples/sdk/observability-basic`](examples/sdk/observability-basic) – observability + inspection

Deeper follow-up examples live in `examples/core`. Published adapter copies run from adapter module roots without build tags:

- [`backend/redis/examples/...`](backend/redis/examples/)
- [`idempotency/redis/examples/...`](idempotency/redis/examples/)

### Running examples

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/sdk/sync-approve-order
```

## Adapters

| Module | Purpose |
|--------|---------|
| [`backend/redis`](backend/redis) | Redis lease backend (standard, strict, lineage) |
| [`idempotency/redis`](idempotency/redis) | Redis idempotency state for async `Claim` |
| [`guard/postgres`](guard/postgres) | Postgres guarded-write helpers |

## Docs

- [`docs/quickstart-sync.md`](docs/quickstart-sync.md)
- [`docs/quickstart-async.md`](docs/quickstart-async.md)
- [`docs/lock-definition-reference.md`](docs/lock-definition-reference.md)
- [`docs/registry-and-usecases.md`](docs/registry-and-usecases.md)
- [`docs/production-guide.md`](docs/production-guide.md)
- [`docs/errors.md`](docs/errors.md)
- [`docs/benchmarks.md`](docs/benchmarks.md)

Advanced: [`docs/advanced/composite.md`](docs/advanced/composite.md) · [`docs/advanced/strict.md`](docs/advanced/strict.md) · [`docs/advanced/lineage.md`](docs/advanced/lineage.md) · [`docs/advanced/guard.md`](docs/advanced/guard.md)

Multiple lock: [`docs/multiple-lock.md`](docs/multiple-lock.md)

## For AI Agents

See [`SKILL.md`](SKILL.md) for a comprehensive reference of all features, APIs, error sentinels, patterns, and example catalog.

## Benchmarks

Contention benchmarks on Apple M4, miniredis-backed (3-run average):

| Benchmark | Parallelism | ns/op | B/op | allocs/op |
|-----------|:-----------:|------:|-----:|----------:|
| **redislock** `Obtain` | 1 | 660,380 | 2,141,529 | 8,982 |
| **lockman** `Run` (distinct owners) | 1 | 219,573 | 221,509 | 1,400 |
| **redislock** `Obtain` | 4 | 2,013,508 | 7,783,079 | 32,757 |
| **lockman** `Run` (distinct owners) | 4 | 278,870 | 277,788 | 2,799 |
| **redislock** `Obtain` | 16 | 5,770,038 | 26,750,397 | 112,706 |
| **lockman** `Run` (distinct owners) | 16 | 906,055 | 533,817 | 9,114 |

`lockman` is **3–7× faster** and uses **6–50× less memory** than direct `redislock` under contention, while keeping allocations an order of magnitude lower.

Run the benchmarks yourself:

```bash
go test -run '^$' -bench 'BenchmarkSyncLock(Redislock|Lockman)Run' -benchmem ./benchmarks
```

See [`docs/benchmarks.md`](docs/benchmarks.md) for methodology and environment notes.

## Status

The root SDK path `github.com/tuanuet/lockman` is the stable entry point for synchronous and asynchronous use-case locking. Adapter modules are versioned as nested Go modules with their own module-path tags.

## Development

```bash
go test ./...
GOWORK=off go test ./...
```
