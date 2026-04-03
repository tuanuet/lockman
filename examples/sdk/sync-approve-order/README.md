# Sync Approve-Order Example

This workspace mirror tracks the public SDK interface. The root `main.go` is gated behind the `lockman_examples` build tag so default root verification stays clean.

## Backbone concept

This example shows the shorthand sync path after the `v1.3.0` definition-first backbone is understood.

## What this example defines

- one shorthand lock definition owned implicitly by `DefineRun`
- one sync execution surface for `order.approve`

The example does not create a named shared definition. It uses shorthand because one `Run` use case is enough for this focused flow.

## Why this shape matters

After learning `DefineLock` plus attached execution surfaces, this is the shortest request/response path on the SDK surface.

It shows where shorthand is enough without replacing the shared-definition backbone as the main model.

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
