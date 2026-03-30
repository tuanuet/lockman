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
- `lockman/redis`: Redis backend constructor for `lockman.WithBackend(...)`
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

- Sync approval: [`redis/examples/sync-order-approval`](redis/examples/sync-order-approval)
- Async processor: [`idempotency/redis/examples/async-order-processor`](idempotency/redis/examples/async-order-processor)
- Composite transfer: [`redis/examples/composite-transfer`](redis/examples/composite-transfer)
- Strict fenced write: [`redis/examples/strict-fenced-write`](redis/examples/strict-fenced-write)

All new examples read `LOCKMAN_REDIS_URL` and default to `redis://127.0.0.1:6379/0`.

Run adapter-backed examples from the adapter module root, for example:

```bash
cd redis
go run ./examples/sync-order-approval
```

Historical engine-first and phase-oriented example directories still exist in `examples/`, but adapter-dependent root Go packages were retired so the root module can validate without sibling adapter modules.

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
