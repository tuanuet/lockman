# Phase 3b Guarded Worker Example

This example demonstrates the Phase 3b strict-worker path where a fencing token is issued by the worker runtime and then enforced again at the Postgres write boundary. Each run uses a unique Postgres table name and drops that table during cleanup so repeated runs do not share state.

## Prerequisites

- Redis running and reachable by the example
- Postgres running and reachable by the example
- A database matching `LOCKMAN_POSTGRES_DSN`
- Permission to create and drop a temporary example table in that database

## Environment Variables

- `LOCKMAN_REDIS_URL`
  Points the example at Redis.
- `LOCKMAN_POSTGRES_DSN`
  Points the example at Postgres through the pgx stdlib driver.

## Run

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 \
LOCKMAN_POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/lockman?sslmode=disable \
go run ./examples/phase3b-guarded-worker
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
