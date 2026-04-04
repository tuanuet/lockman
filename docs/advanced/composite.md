# Advanced: Composite

Use `github.com/tuanuet/lockman/advanced/composite` when one synchronous operation must hold multiple resources together.

## When To Use It

- transfer operations
- two-resource consistency boundaries
- cases where one callback should start only after multiple resources are acquired

## Public Shape

```go
accountDef := lockman.DefineLock(
	"account",
	lockman.BindResourceID("account", func(in Input) string { return in.AccountID }),
)

ledgerDef := lockman.DefineLock(
	"ledger",
	lockman.BindResourceID("ledger", func(in Input) string { return in.LedgerID }),
)

transferDef := composite.DefineLock("transfer", accountDef, ledgerDef)
transfer := composite.AttachRun("transfer.run", transferDef, lockman.TTL(5*time.Second))
```

The child definitions may stay private inside one package. Reuse is available when
it helps your model, but it is not required.

Then register it in the normal root registry and execute it with the normal root client:

```go
req, _ := transfer.With(Input{AccountID: "acct-123", LedgerID: "ledger-456"})
err := client.Run(ctx, req, func(ctx context.Context, lease lockman.Lease) error {
	log.Println(lease.ResourceKeys)
	return nil
})
```

## Fail-If-Held Members

Mark a member definition with `lockman.FailIfHeldDef()` to turn it into a check-only
precondition. The composite will abort immediately with `lockman.ErrPreconditionFailed`
if that resource is already held, without acquiring any other members.

```go
preconditionDef := lockman.DefineLock(
	"precondition",
	lockman.BindResourceID("account", func(in Input) string { return in.AccountID }),
	lockman.FailIfHeldDef(),
)

ledgerDef := lockman.DefineLock(
	"ledger",
	lockman.BindResourceID("ledger", func(in Input) string { return in.LedgerID }),
)

transferDef := composite.DefineLock("transfer", preconditionDef, ledgerDef)
```

- `FailIfHeldDef()` members are checked before any acquire begins
- If a check-only member is held, the composite returns `ErrPreconditionFailed`
- Check-only members are excluded from the callback `Lease.ResourceKeys`, guard tracking, and active-lock metrics
- `FailIfHeldDef()` can be combined with `StrictDef()` on the same definition

Runnable examples:

- Workspace SDK mirror: [`examples/sdk/sync-transfer-funds`](../../examples/sdk/sync-transfer-funds)
- Published adapter copy: [`backend/redis/examples/sync-transfer-funds`](../../backend/redis/examples/sync-transfer-funds)
