# Sync Approve-Order Example

This preserved `examples/core` copy is the source material for the public-SDK workspace mirror in `examples/sdk`. It stays gated behind the `lockman_examples` build tag so default root verification stays clean.

Run the preserved core copy from the workspace root:

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/core/sync-approve-order
```

If you want the public SDK-oriented workspace path first, use `./examples/sdk/sync-approve-order`.

Published adapter runnable path:

```bash
cd backend/redis
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/sync-approve-order
```
