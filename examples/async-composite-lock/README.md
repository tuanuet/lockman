# Async Composite Lock Example

This example source is kept in the root workspace. Its `main.go` is gated behind the `lockman_examples` build tag so default root verification does not depend on sibling adapter modules.

This example is the async counterpart to the sync composite example. It combines composite locking with worker-style idempotent claim execution.

## What It Shows

- `workers.ExecuteCompositeClaimed`
- Redis-backed composite lease acquisition
- Redis-backed idempotency completion for one async composite job
- Canonical ordering of composite member resource keys in worker callbacks

## Status

- This remains a runnable workspace example.
- It intentionally uses the lower-level `registry` and `workers` APIs because it demonstrates an advanced composite worker lifecycle.
- If you want the default user-facing API first, start with [`docs/quickstart-async.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/quickstart-async.md) and [`docs/advanced/composite.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/advanced/composite.md).

## Run

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/async-composite-lock
```

## Flow

1. Register two async member definitions: `AccountMember` and `LedgerMember`.
2. Register one async composite definition named `TransferComposite`.
3. Execute a composite claim request with one idempotency key.
4. Inside the callback, inspect the ordered list of composite resource keys.
5. After the callback returns, inspect the persisted idempotency record.

## Output To Notice

- `composite callback: account:acct-123,ledger:ledger-456`
  This proves the worker acquired the whole composite and entered the callback with canonical ordering.
- `composite idempotency after ack: completed`
  This proves the composite worker path persisted terminal idempotency state after success.
- `shutdown: ok`
  This proves the worker manager shut down cleanly after the claim completed.

## When To Use This Example

Use this when one queue message must atomically claim multiple resources before the handler runs.
