# Async Process-Order Example

This workspace mirror tracks the public SDK interface. The root `main.go` is gated behind the `lockman_examples` build tag so default root verification stays clean.

## Backbone concept

This example shows the deprecated shorthand async path after the `v1.3.0` definition-first backbone is understood.

## What this example defines

- one shorthand lock definition owned implicitly by `DefineClaim`
- one async execution surface for `order.process`
- idempotent delivery handling on the SDK path

The example stays on the deprecated shorthand constructor because this workspace mirror still covers the compatibility path that remains fully functional in the current release line.

## Why this shape matters

After the shared-definition backbone is clear, this example shows how older shorthand-based claim flows map onto the current SDK surface.

It is compatibility coverage for existing users, not the recommended starting point for new code.

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
