# Sync Approve-Order Example

This workspace mirror tracks the public SDK interface. The root `main.go` is gated behind the `lockman_examples` build tag so default root verification stays clean.

Run the SDK workspace mirror from the workspace root:

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/sdk/sync-approve-order
```

Published adapter runnable path:

```bash
cd backend/redis
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/sync-approve-order
```
