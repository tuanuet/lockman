# Async Process-Order (Redis Idempotency Adapter Example)

This example wires both:

- `github.com/tuanuet/lockman/backend/redis` as the backend
- `github.com/tuanuet/lockman/idempotency/redis` as the idempotency store

## Run

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/async-process-order
```
