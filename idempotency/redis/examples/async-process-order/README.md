# Async Process-Order (Redis Idempotency Adapter Example)

This example wires both:

- `lockman/redis` as the backend
- `lockman/idempotency/redis` as the idempotency store

## Run

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/async-process-order
```
