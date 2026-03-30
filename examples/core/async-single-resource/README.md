# Async Single-Resource Example

This example source is kept in the root workspace. Its `main.go` is gated behind the `lockman_examples` build tag so default root verification does not depend on sibling adapter modules.

This example is the shortest path to understand async single-resource worker locking.

## What It Shows

- `workers.ExecuteClaimed` on one parent lock definition
- Redis-backed lease management
- Redis-backed idempotency store
- Duplicate message suppression after the first successful completion
- Presence checks while the lease is held and after it is released

## Status

- This remains a runnable workspace example.
- It intentionally uses the lower-level `registry` and `workers` APIs because it demonstrates an advanced worker lifecycle.
- If you want the default user-facing API first, start with [`docs/quickstart-async.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/quickstart-async.md).

## Run

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/core/async-single-resource
```

## Flow

1. Register one async lock definition named `OrderClaim`.
2. Build a worker manager with the Redis lease driver and Redis idempotency store.
3. Execute one message claim for `order:123`.
4. Inside the callback, inspect presence for the same resource key.
5. After the callback returns, inspect the idempotency record and lease presence again.
6. Re-run the same message claim and observe duplicate suppression.

## Output To Notice

- `execute: callback running for order:123`
  This proves the first claim actually entered the callback.
- `presence while held: held`
  This proves the exact lease exists while the callback is running.
- `idempotency after ack: completed`
  This proves the async path persisted a completed terminal idempotency record.
- `presence after release: not_held`
  This proves the lease was released after callback completion.
- `duplicate outcome: ignored`
  This proves the same idempotency key is not processed twice.

## When To Use This Example

Start here if your system looks like "one queue message maps to one resource key and one worker callback".
