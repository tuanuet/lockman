# Examples

From `v1.3.0`, the SDK examples follow the same definition-first model as the root README.

The root example tree is split into two layers:

- `examples/sdk`: workspace mirrors of the current public SDK interface
- `examples/core`: preserved scenario examples and lower-level teaching flows

If you are new to the project, start with `examples/sdk`.

## Start Here

- `examples/sdk/shared-lock-definition`

This is the canonical first example for the `v1.3.0` SDK path. It teaches the backbone directly:

- create a lock definition first
- attach execution surfaces to it
- register those use cases once

## Choose An Execution Surface

- `examples/sdk/sync-approve-order`
- `examples/sdk/async-process-order`

These show focused definition-first examples once the shared-definition backbone is clear:

- `sync-approve-order` covers the shortest sync `Run` flow built on `DefineLock + DefineRunOn`
- `async-process-order` covers the shortest async `Claim` flow built on `DefineLock + DefineClaimOn`

## Shared Definition Patterns

- `examples/sdk/shared-aggregate-split-definitions`
- `examples/sdk/parent-lock-over-composite`
- `examples/sdk/sync-transfer-funds`

These extend the backbone into modeling choices across aggregate boundaries.

They stay on the same definition-first SDK model while showing different ways to choose and share lock boundaries.

## Advanced Coordination

- `examples/sdk/sync-fenced-write`
- `examples/sdk/observability-basic`

`sync-fenced-write` keeps the same SDK authoring model but layers stricter execution semantics on top.

Published adapter-backed runnable copies still live in:

- `backend/redis/examples/...`
- `idempotency/redis/examples/...`

## About examples/core

`examples/core` is preserved deeper material.

Use it after the `examples/sdk` path when you want lower-level scenario framing, older teaching flows, or deeper follow-up examples that are not the main public SDK learning path.

Workspace SDK mirrors are gated behind the `lockman_examples` build tag:

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/sdk/sync-approve-order
```
