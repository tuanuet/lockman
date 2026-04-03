# Sync Approve-Order Example

This workspace mirror tracks the public SDK interface. The root `main.go` is gated behind the `lockman_examples` build tag so default root verification stays clean.

## Backbone concept

This example shows the shortest sync path on the `v1.3.0` definition-first backbone.

## What this example defines

- one named lock definition: `orderDef`
- one sync execution surface for `order.approve`

The example keeps the smallest recommended SDK shape for new code: define the boundary once, then attach a sync surface to it.

## Why this shape matters

After learning the shared-definition backbone, this is the smallest runnable request/response flow on that same model.

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
