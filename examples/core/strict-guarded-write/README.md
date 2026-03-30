# Strict Guarded-Write Example

This example source is kept in the root workspace. Its `main.go` is gated behind the `lockman_examples` build tag so default root verification does not depend on sibling adapter modules.

This example demonstrates the strict worker path where a fencing token is issued by the worker runtime and then enforced again at the Postgres write boundary. Each run uses a unique Postgres table name and drops that table during cleanup so repeated runs do not share state.

## Status

- This remains a runnable workspace example.
- It intentionally uses the lower-level `registry` and `workers` APIs because it demonstrates advanced strict worker execution plus guarded persistence.
- If you want the default user-facing API first, start with [`docs/advanced/strict.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/advanced/strict.md) and [`docs/advanced/guard.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/advanced/guard.md).

## Run

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 \
LOCKMAN_POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/lockman?sslmode=disable \
go run -tags lockman_examples ./examples/core/strict-guarded-write
```

## Output Meaning

- `first worker claim token: 1`
  The first strict async claim received fencing token `1`.
- `first guarded outcome: applied`
  The first database update used token `1` and committed.
- `second worker claim token: 2`
  A later strict claim on the same boundary received a newer fencing token.
- `second guarded outcome: applied`
  The second database update succeeded because token `2` is newer than token `1`.
- `late stale outcome: stale_rejected`
  Reusing the first claim's guard context after token `2` committed is rejected at the database boundary.
- `idempotency after ack: completed`
  The successful second worker callback still records terminal worker idempotency state.
- `teaching point: phase3b carries the strict fencing token into the database write path`
  Phase 3b completes the persistence-side half of strict mode for this example.
- `shutdown: ok`
  The worker manager shut down cleanly after the example finished.
