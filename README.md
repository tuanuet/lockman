# lockman

`lockman` is a typed Go SDK for locking business use cases with one simple path for sync, hold, and async workflows.

From `v1.3.0`, the public SDK story is definition-first:

- define one lock boundary first
- attach one or more execution surfaces to it
- register centrally
- call `Run`, `Hold`, or `Claim`

## Why lockman

- You bind typed input to a lock definition instead of building lock keys by hand at callsites.
- Sync, hold, and async flows can share one business boundary instead of feeling like separate products.
- The happy path stays short, but stricter coordination features are still available when you need them.

## The SDK Backbone

The `v1.3.0` SDK model is definition-first:

1. Create a shared lock definition with `DefineLock`.
2. Attach one or more execution surfaces with `DefineRunOn`, `DefineHoldOn`, or `DefineClaimOn`.
3. Register those use cases in one registry.
4. Execute through the root client.

This is the public SDK path the README and `examples/sdk` now optimize for.

## Install

Install the root SDK module with:

```bash
go get github.com/tuanuet/lockman
```

## Definition-First Happy Path

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

var ManualHold = lockman.DefineHoldOn("order.manual_hold", OrderDef)

func approve(ctx context.Context, redisClient any) error {
	reg := lockman.NewRegistry()
	if err := reg.Register(Approve, ManualHold); err != nil {
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

The point of this example is that one `LockDefinition` owns the shared business boundary, while attached execution surfaces share that same boundary.

If you want the smallest runnable version of that model, start with [`examples/sdk/shared-lock-definition`](examples/sdk/shared-lock-definition).

## Deprecated Shorthand Constructors

`DefineRun`, `DefineHold`, and `DefineClaim` are deprecated.

They remain fully functional in the current release line for compatibility, but new code should use `DefineLock` plus `DefineRunOn`, `DefineHoldOn`, or `DefineClaimOn`.

The next major release will remove these shorthand constructors from the root SDK.

Mechanical migration looks like this:

```go
// before
var Approve = lockman.DefineRun[ApproveInput](
	"order.approve",
	lockman.BindResourceID("order", func(in ApproveInput) string { return in.OrderID }),
)

// after
var OrderDef = lockman.DefineLock(
	"order",
	lockman.BindResourceID("order", func(in ApproveInput) string { return in.OrderID }),
)

var Approve = lockman.DefineRunOn("order.approve", OrderDef)
```

The extracted definition can stay private to one package. You do not need shared multi-use-case coordination to justify this migration; the point is one consistent public authoring model.

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

## Examples By Learning Path

Start here:

- [`examples/sdk/shared-lock-definition`](examples/sdk/shared-lock-definition): the canonical first example for the `v1.3.0` definition-first SDK path

Then choose an execution surface:

- [`examples/sdk/sync-approve-order`](examples/sdk/sync-approve-order): the shortest sync request/response flow on the SDK path
- [`examples/sdk/async-process-order`](examples/sdk/async-process-order): the shortest async delivery flow with idempotency on the SDK path

Then expand into shared-definition patterns:

- [`examples/sdk/shared-aggregate-split-definitions`](examples/sdk/shared-aggregate-split-definitions): compare sync and async flows over one aggregate boundary
- [`examples/sdk/parent-lock-over-composite`](examples/sdk/parent-lock-over-composite): when one aggregate boundary is enough and composite locking is overkill
- [`examples/sdk/sync-transfer-funds`](examples/sdk/sync-transfer-funds): one operation holding multiple resources together

Advanced coordination on the same SDK path:

- [`examples/sdk/sync-fenced-write`](examples/sdk/sync-fenced-write): strict fenced execution on top of the same authoring model

Some scenarios intentionally appear in both `examples/core` and `examples/sdk`.

New SDK readers should start in `examples/sdk`.

`examples/sdk` keeps workspace mirrors of the current public SDK interface. `examples/core` keeps preserved lower-level source material for deeper follow-up study once the public path is clear.

Additional deeper follow-up examples in `examples/core` include:

- [`examples/core/strict-guarded-write`](examples/core/strict-guarded-write): strict fencing carried all the way into a guarded database write
- [`examples/core/parent-lock-over-composite`](examples/core/parent-lock-over-composite): lower-level preserved scenario framing for aggregate-over-composite reasoning

Published adapter-backed copies also live here:

- [`backend/redis/examples/sync-approve-order`](backend/redis/examples/sync-approve-order)
- [`backend/redis/examples/sync-transfer-funds`](backend/redis/examples/sync-transfer-funds)
- [`backend/redis/examples/sync-fenced-write`](backend/redis/examples/sync-fenced-write)
- [`idempotency/redis/examples/async-process-order`](idempotency/redis/examples/async-process-order)

## Run Or Claim?

- Use `Run` for synchronous critical sections in request/response or job orchestration flows.
- Use `Hold` when a user or process needs to retain a manual lock over a shared definition boundary.
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

The root SDK path `github.com/tuanuet/lockman` is the stable entry point for synchronous and asynchronous use-case locking. Adapter modules such as `backend/redis`, `idempotency/redis`, and `guard/postgres` are versioned as nested Go modules with their own module-path tags.

## Development

```bash
go test ./...
GOWORK=off go test ./...
```
