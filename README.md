# lockman

`lockman` is a Go SDK for registry-backed distributed locking with one default user path:

1. define a use case
2. register it centrally
3. create a client
4. call `Run` or `Claim`

## Install

This repository is still pre-release. The in-repo module path is currently:

```bash
go get lockman
```

## Start Here

- Sync quickstart: [`docs/quickstart-sync.md`](docs/quickstart-sync.md)
- Async quickstart: [`docs/quickstart-async.md`](docs/quickstart-async.md)
- Registry and use cases: [`docs/registry-and-usecases.md`](docs/registry-and-usecases.md)
- `Run` vs `Claim`: [`docs/runtime-vs-workers.md`](docs/runtime-vs-workers.md)
- Error guide: [`docs/errors.md`](docs/errors.md)
- Definition reference: [`docs/lock-definition-reference.md`](docs/lock-definition-reference.md)
- Best practices: [`docs/lock-scenarios-and-best-practices.md`](docs/lock-scenarios-and-best-practices.md)

## Public Packages

- `lockman`: default client, registry, use cases, `Run`, and `Claim`
- `lockman/backend/redis`: Redis backend constructor for `lockman.WithBackend(...)`
- `lockman/idempotency/redis`: Redis idempotency store for `lockman.WithIdempotency(...)`
- `lockman/advanced/composite`: advanced composite run authoring
- `lockman/advanced/strict`: advanced strict fenced run authoring
- `lockman/advanced/lineage`: reserved advanced namespace for lineage-oriented flows
- `lockman/advanced/guard`: reserved advanced namespace for guarded-write integrations

## Quick Example

```go
package orderlocks

import "lockman"

type ApproveInput struct {
	OrderID string
}

var Approve = lockman.DefineRun[ApproveInput](
	"order.approve",
	lockman.BindResourceID("order", func(in ApproveInput) string { return in.OrderID }),
)
```

```go
reg := lockman.NewRegistry()
if err := reg.Register(orderlocks.Approve); err != nil {
	return err
}

client, err := lockman.New(
	lockman.WithRegistry(reg),
	lockman.WithIdentity(lockman.Identity{OwnerID: "orders-api"}),
	lockman.WithBackend(redis.New(redisClient, "")),
)
if err != nil {
	return err
}
defer client.Shutdown(ctx)

req, err := orderlocks.Approve.With(orderlocks.ApproveInput{OrderID: "123"})
if err != nil {
	return err
}

return client.Run(ctx, req, func(ctx context.Context, lease lockman.Lease) error {
	return approveOrder(ctx, "123")
})
```

## Examples

- Workspace guide: [`examples/README.md`](examples/README.md)
- SDK mirror, sync approve order: [`examples/sdk/sync-approve-order`](examples/sdk/sync-approve-order)
- SDK mirror, async process order: [`examples/sdk/async-process-order`](examples/sdk/async-process-order)
- SDK mirror, shared aggregate split definitions: [`examples/sdk/shared-aggregate-split-definitions`](examples/sdk/shared-aggregate-split-definitions)
- SDK mirror, parent lock over composite: [`examples/sdk/parent-lock-over-composite`](examples/sdk/parent-lock-over-composite)
- SDK mirror, sync transfer funds: [`examples/sdk/sync-transfer-funds`](examples/sdk/sync-transfer-funds)
- SDK mirror, sync fenced write: [`examples/sdk/sync-fenced-write`](examples/sdk/sync-fenced-write)
- Published adapter copy, sync approve order: [`backend/redis/examples/sync-approve-order`](backend/redis/examples/sync-approve-order)
- Published adapter copy, async process order: [`idempotency/redis/examples/async-process-order`](idempotency/redis/examples/async-process-order)
- Published adapter copy, sync transfer funds: [`backend/redis/examples/sync-transfer-funds`](backend/redis/examples/sync-transfer-funds)
- Published adapter copy, sync fenced write: [`backend/redis/examples/sync-fenced-write`](backend/redis/examples/sync-fenced-write)

All new examples read `LOCKMAN_REDIS_URL` and default to `redis://127.0.0.1:6379/0`.

Run the workspace SDK mirror from the repo root:

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/sdk/sync-approve-order
```

Run the published adapter-backed copy from the adapter module root:

```bash
cd backend/redis
go run ./examples/sync-approve-order
```

Lower-level and scenario-heavy workspace examples now live under `examples/core/`. They intentionally keep their lower-level `registry`, `runtime`, or `workers` setup where that better teaches the scenario.

## Advanced Cases

Stay on the root `lockman` path unless you need something explicitly advanced:

- composite locking: [`docs/advanced/composite.md`](docs/advanced/composite.md)
- strict fenced execution: [`docs/advanced/strict.md`](docs/advanced/strict.md)
- lineage notes: [`docs/advanced/lineage.md`](docs/advanced/lineage.md)
- guard notes: [`docs/advanced/guard.md`](docs/advanced/guard.md)

## Verification

```bash
go test ./...
go test ./... -cover
```
