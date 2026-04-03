# Async Process-Order Example

This workspace mirror tracks the public SDK interface. The root `main.go` is gated behind the `lockman_examples` build tag so default root verification stays clean.

## Backbone concept

This example shows the shortest async worker path on the `v1.3.0` definition-first backbone.

## What this example defines

- one named lock definition: `orderDef`
- one async execution surface for `order.process`
- idempotent delivery handling on the SDK path

The example keeps the recommended SDK shape for new async code: define the boundary once, then attach an idempotent claim surface to it.

## Why this shape matters

After the shared-definition backbone is clear, this is the smallest runnable message-driven flow on that same model.

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
