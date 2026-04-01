# Examples

The root example tree is split into two layers:

- `examples/sdk`: workspace mirrors of the current public SDK interface
- `examples/core`: preserved scenario examples and lower-level teaching flows

If you are new to the project, start with `examples/sdk`.

Current SDK mirrors:

- `examples/sdk/sync-approve-order`
- `examples/sdk/async-process-order`
- `examples/sdk/shared-aggregate-split-definitions`
- `examples/sdk/parent-lock-over-composite`
- `examples/sdk/sync-transfer-funds`
- `examples/sdk/sync-fenced-write`
- `examples/sdk/observability-basic`

Published adapter-backed runnable copies still live in:

- `backend/redis/examples/...`
- `idempotency/redis/examples/...`

Workspace SDK mirrors are gated behind the `lockman_examples` build tag:

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/sdk/sync-approve-order
```
