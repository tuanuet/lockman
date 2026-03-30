# Advanced: Composite

Use `lockman/advanced/composite` when one synchronous operation must hold multiple resources together.

## When To Use It

- transfer operations
- two-resource consistency boundaries
- cases where one callback should start only after multiple resources are acquired

## Public Shape

```go
transfer := composite.DefineRunWithOptions(
	"transfer.run",
	[]lockman.UseCaseOption{lockman.TTL(5 * time.Second)},
	composite.DefineMember("account", lockman.BindResourceID("account", func(in Input) string { return in.AccountID })),
	composite.DefineMember("ledger", lockman.BindResourceID("ledger", func(in Input) string { return in.LedgerID })),
)
```

Then register it in the normal root registry and execute it with the normal root client:

```go
req, _ := transfer.With(Input{AccountID: "acct-123", LedgerID: "ledger-456"})
err := client.Run(ctx, req, func(ctx context.Context, lease lockman.Lease) error {
	log.Println(lease.ResourceKeys)
	return nil
})
```

Runnable example: [`redis/examples/composite-transfer`](../../redis/examples/composite-transfer)
