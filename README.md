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
- Child overlap rejection and standard-mode-only composite execution (Phase 2 reject-first overlap policy)
- Lock definition field reference: [`docs/lock-definition-reference.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-definition-reference.md)

## Redis Verification

Redis integration tests read `LOCKMAN_REDIS_URL` and skip when unset.

```bash
docker compose up -d redis
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./lockkit/drivers/redis ./lockkit/idempotency/redis -v
```

If `6379` is already in use:

```bash
LOCKMAN_REDIS_PORT=6380 docker compose up -d redis
LOCKMAN_REDIS_URL=redis://localhost:6380/0 go run ./examples/phase2-basic
```

## Phase 2 Examples

Redis-backed:

- `go run ./examples/phase2-basic`
- `go run ./examples/phase2-composite-worker`

Memory-backed:

- `go run ./examples/phase2-composite-sync`
- `go run ./examples/phase2-overlap-reject`

## Dependency Boundaries

- `go run ./examples/reentrant` shows nested acquire rejection is a reentrant guard, not dependency analysis.
- `go run ./examples/no-dependency-awareness` shows a child-like lock with `ParentRef` can still nest under a parent lock because Phase 1 does not enforce parent-child dependency semantics.

## Commands

- `go test ./...`
- `go test ./... -cover`
- `go run ./examples/basic`
- `go run ./examples/phase2-basic`
- `go run ./examples/phase2-composite-sync`
- `go run ./examples/phase2-composite-worker`
- `go run ./examples/phase2-overlap-reject`
- `go run ./examples/contention`
- `go run ./examples/no-dependency-awareness`
- `go run ./examples/reentrant`
- `go run ./examples/ttl`
