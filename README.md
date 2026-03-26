# lockman

Distributed lock platform SDK prototype for Go.

## Phase 1 Status

- Standard-mode exclusive execution via `ExecuteExclusive`
- Advisory presence checks via `CheckPresence`
- Lifecycle shutdown via `Shutdown(ctx)`
- Central registry validation plus the in-memory `testkit` driver
- Parent-lock focused scope with baseline runtime metrics

## Phase 2 Status

- Worker claim execution via `ExecuteClaimed` and `ExecuteCompositeClaimed`
- Redis production driver and Redis-backed idempotency store
- Child overlap rejection and standard composite execution

## Redis Verification

Redis integration tests read `LOCKMAN_REDIS_URL` and skip when unset.

```bash
docker run --rm -p 6379:6379 redis:7-alpine
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./lockkit/drivers/redis ./lockkit/idempotency/redis -v
```

## Dependency Boundaries

- `go run ./examples/reentrant` shows nested acquire rejection is a reentrant guard, not dependency analysis.
- `go run ./examples/no-dependency-awareness` shows a child-like lock with `ParentRef` can still nest under a parent lock because Phase 1 does not enforce parent-child dependency semantics.

## Commands

- `go test ./...`
- `go test ./... -cover`
- `go run ./examples/basic`
- `go run ./examples/contention`
- `go run ./examples/no-dependency-awareness`
- `go run ./examples/reentrant`
- `go run ./examples/ttl`
