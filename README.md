# lockman

`lockman` is a typed Go SDK for locking business use cases with one simple path for both sync and async workflows.

- define a use case once
- register it centrally
- call `Run` or `Claim`

## Why It Feels Simple

- You bind typed input to a use case instead of building lock keys by hand at callsites.
- Sync and async flows share the same mental model instead of feeling like two separate products.
- The happy path stays short, but stricter coordination features are still available when you need them.

## Install

Install the root SDK module with:

```bash
go get github.com/tuanuet/lockman
```

## Happy Path

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

var Approve = lockman.DefineRun[ApproveInput](
	"order.approve",
	lockman.BindResourceID("order", func(in ApproveInput) string { return in.OrderID }),
)

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

If you want the smallest runnable version of that flow, start with [`examples/sdk/sync-approve-order`](examples/sdk/sync-approve-order).

## Shared Lock Definitions

Use `LockDefinition` when multiple use cases should contend on the same lock identity.

- `DefineLock(...)` owns the shared lock identity, binding, and strictness configuration.
- `DefineRunOn(...)`, `DefineHoldOn(...)`, and `DefineClaimOn(...)` attach execution surfaces to that shared definition.
- `DefinitionID()` on use cases remains public-name-facing for compatibility and observability.

```go
contractDef := lockman.DefineLock(
	"order.contract",
	lockman.BindResourceID("order", func(in OrderInput) string { return in.OrderID }),
)

importUC := lockman.DefineRunOn("order.import", contractDef)
holdUC := lockman.DefineHoldOn("order.manual_hold", contractDef)
```

See [`examples/sdk/shared-lock-definition`](examples/sdk/shared-lock-definition) for a focused runnable example.

## Examples By Use Case

- [`examples/sdk/sync-approve-order`](examples/sdk/sync-approve-order): the shortest sync request/response flow on the SDK path
- [`examples/sdk/async-process-order`](examples/sdk/async-process-order): the shortest async delivery flow with idempotency
- [`examples/sdk/sync-transfer-funds`](examples/sdk/sync-transfer-funds): one operation holding multiple resources together
- [`examples/sdk/sync-fenced-write`](examples/sdk/sync-fenced-write): strict fenced execution on the SDK path
- [`examples/sdk/shared-aggregate-split-definitions`](examples/sdk/shared-aggregate-split-definitions): compare sync and async flows on one aggregate boundary
- [`examples/sdk/shared-lock-definition`](examples/sdk/shared-lock-definition): share one lock definition across run and hold use cases
- [`examples/core/strict-guarded-write`](examples/core/strict-guarded-write): strict fencing carried all the way into a guarded database write
- [`examples/core/parent-lock-over-composite`](examples/core/parent-lock-over-composite): when one aggregate lock is enough and composite locking is overkill

Some scenarios intentionally appear in both `examples/core` and `examples/sdk`.
`examples/core` keeps preserved lower-level source material, while `examples/sdk` keeps workspace mirrors of the current public SDK interface.

Published adapter-backed copies also live here:

- [`backend/redis/examples/sync-approve-order`](backend/redis/examples/sync-approve-order)
- [`backend/redis/examples/sync-transfer-funds`](backend/redis/examples/sync-transfer-funds)
- [`backend/redis/examples/sync-fenced-write`](backend/redis/examples/sync-fenced-write)
- [`idempotency/redis/examples/async-process-order`](idempotency/redis/examples/async-process-order)

## Run Or Claim?

- Use `Run` for synchronous critical sections in request/response or job orchestration flows.
- Use `Claim` when work starts from delivery, retry, or redelivery semantics and needs idempotent processing.

More detail:

- [`docs/production-guide.md`](docs/production-guide.md) — production checklist, wiring patterns, and which example to copy
- [`docs/benchmarks.md`](docs/benchmarks.md) — benchmark tracks for calibrating SDK and adapter overhead
- [`docs/quickstart-sync.md`](docs/quickstart-sync.md)
- [`docs/quickstart-async.md`](docs/quickstart-async.md)
- [`docs/runtime-vs-workers.md`](docs/runtime-vs-workers.md)
- [`docs/errors.md`](docs/errors.md)

## When You Need More

- Composite locking: [`docs/advanced/composite.md`](docs/advanced/composite.md)
- Strict fenced execution: [`docs/advanced/strict.md`](docs/advanced/strict.md)
- Lineage and overlap rules: [`docs/advanced/lineage.md`](docs/advanced/lineage.md)
- Guarded write integrations: [`docs/advanced/guard.md`](docs/advanced/guard.md)
- Registry patterns and use case authoring: [`docs/registry-and-usecases.md`](docs/registry-and-usecases.md)

## Status

`lockman` `v1.0.0` is released.

The root SDK path `github.com/tuanuet/lockman` is the stable entry point for synchronous and asynchronous use-case locking. Adapter modules such as `backend/redis`, `idempotency/redis`, and `guard/postgres` are versioned as nested Go modules with their own module-path tags.

## Development

```bash
go test ./...
GOWORK=off go test ./...
```
