# Quickstart: Async

Use `Claim` when a delivery may be retried or redelivered.

```go
package orderlocks

import "lockman"

type ProcessInput struct {
	OrderID string
}

var Process = lockman.DefineClaim[ProcessInput](
	"order.process",
	lockman.BindResourceID("order", func(in ProcessInput) string { return in.OrderID }),
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

Runnable example: [`examples/async-order-processor`](../examples/async-order-processor)
