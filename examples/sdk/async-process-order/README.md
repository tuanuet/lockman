# Async Process-Order Example

This workspace mirror tracks the public SDK interface. The root `main.go` is gated behind the `lockman_examples` build tag so default root verification stays clean.

## Backbone concept

This example shows the shorthand async path after the `v1.3.0` definition-first backbone is understood.

## What this example defines

- one shorthand lock definition owned implicitly by `DefineClaim`
- one async execution surface for `order.process`
- idempotent delivery handling on the SDK path

The example stays on shorthand because it teaches one focused `Claim` flow rather than a shared definition across multiple public use cases.

## Why this shape matters

After the shared-definition backbone is clear, this is the shortest way to see async claiming, delivery metadata, and duplicate handling together.

It shows that shorthand is still useful, while shared lock definition remains the primary SDK authoring model.

## How to run

Run the SDK workspace mirror from the workspace root:

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/sdk/async-process-order
```

Published adapter runnable path:

```bash
cd idempotency/redis
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/async-process-order
```
