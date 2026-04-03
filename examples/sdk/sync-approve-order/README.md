# Sync Approve-Order Example

This workspace mirror tracks the public SDK interface. The root `main.go` is gated behind the `lockman_examples` build tag so default root verification stays clean.

## Backbone concept

This example shows the deprecated shorthand sync path after the `v1.3.0` definition-first backbone is understood.

## What this example defines

- one shorthand lock definition owned implicitly by `DefineRun`
- one sync execution surface for `order.approve`

The example does not create a named shared definition. It uses the deprecated shorthand constructor because this workspace mirror still covers the compatibility path that remains fully functional in the current release line.

## Why this shape matters

After learning `DefineLock` plus attached execution surfaces, this example shows how older shorthand code maps onto the current SDK surface.

It is compatibility coverage for existing users, not the recommended starting point for new code.

## How to run

Run the SDK workspace mirror from the workspace root:

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/sdk/sync-approve-order
```

Published adapter runnable path:

```bash
cd backend/redis
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/sync-approve-order
```
