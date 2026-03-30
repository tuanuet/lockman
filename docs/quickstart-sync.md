# Quickstart: Sync

This is the default synchronous path.

1. define a typed use case once
2. register it centrally at startup
3. bind typed input and call `Client.Run(...)`

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
```

```go
req, err := orderlocks.Approve.With(orderlocks.ApproveInput{OrderID: "123"})
if err != nil {
	return err
}

err = client.Run(ctx, req, func(ctx context.Context, lease lockman.Lease) error {
	return approveOrder(ctx, "123")
})
```

Runnable example: [`backend/redis/examples/sync-approve-order`](../backend/redis/examples/sync-approve-order)

Run it from the adapter module root:

```bash
cd backend/redis
go run ./examples/sync-approve-order
```
