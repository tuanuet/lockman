# Quickstart: Async

Use `Claim` when a delivery may be retried or redelivered.

From `v1.3.0`, the SDK backbone is still definition-first on the async path: define the lock boundary first, then attach the async execution surface.

```go
package orderlocks

import "github.com/tuanuet/lockman"

type ProcessInput struct {
	OrderID string
}

var OrderDef = lockman.DefineLock(
	"order",
	lockman.BindResourceID("order", func(in ProcessInput) string { return in.OrderID }),
)

var Process = lockman.DefineClaimOn(
	"order.process",
	OrderDef,
	lockman.Idempotent(),
)
```

```go
reg := lockman.NewRegistry()
if err := reg.Register(orderlocks.Process); err != nil {
	return err
}

client, err := lockman.New(
	lockman.WithRegistry(reg),
	lockman.WithIdentity(lockman.Identity{OwnerID: "orders-worker"}),
	lockman.WithBackend(redis.New(redisClient, "")),
	lockman.WithIdempotency(idempotencyredis.New(redisClient, "")),
)
if err != nil {
	return err
}
defer client.Shutdown(ctx)
```

```go
req, err := orderlocks.Process.With(
	orderlocks.ProcessInput{OrderID: "123"},
	lockman.Delivery{
		MessageID:     "msg-1",
		ConsumerGroup: "orders",
		Attempt:       1,
	},
)
if err != nil {
	return err
}

err = client.Claim(ctx, req, func(ctx context.Context, claim lockman.Claim) error {
	return processOrder(ctx, "123")
})
```

Runnable examples:

- Definition-first SDK path: [`examples/sdk/shared-lock-definition`](../examples/sdk/shared-lock-definition)
- Workspace SDK mirror: [`examples/sdk/async-process-order`](../examples/sdk/async-process-order)
- Published adapter copy: [`idempotency/redis/examples/async-process-order`](../idempotency/redis/examples/async-process-order)

If you only need one focused async use case, the shorthand `DefineClaim(...)` form shown in `examples/sdk/async-process-order` is still valid. Use it as a convenience path after the shared-definition backbone is clear.

Run the workspace SDK mirror from the repo root:

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/sdk/async-process-order
```

Or run the published adapter copy from the adapter module root:

```bash
cd idempotency/redis
go run ./examples/async-process-order
```
