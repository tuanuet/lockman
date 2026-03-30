# Advanced: Strict

Use `github.com/tuanuet/lockman/advanced/strict` when a synchronous critical section needs fencing tokens.

## When To Use It

- stale writer protection
- compare-and-swap or guarded persistence
- integrations that must observe monotonic fencing tokens

## Public Shape

```go
approve := strict.DefineRun(
	"order.strict-write",
	lockman.BindResourceID("order", func(in Input) string { return in.OrderID }),
)
```

The resulting use case still runs through the normal root client:

```go
req, _ := approve.With(Input{OrderID: "123"})
err := client.Run(ctx, req, func(ctx context.Context, lease lockman.Lease) error {
	log.Println(lease.FencingToken)
	return nil
})
```

Runnable examples:

- Workspace SDK mirror: [`examples/sdk/sync-fenced-write`](../../examples/sdk/sync-fenced-write)
- Published adapter copy: [`backend/redis/examples/sync-fenced-write`](../../backend/redis/examples/sync-fenced-write)
