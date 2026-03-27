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
- Runtime vs workers guide: [`docs/runtime-vs-workers.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/runtime-vs-workers.md)

## Phase 2a Status

- `ExecuteExclusive` and `ExecuteClaimed` now enforce parent-child overlap across goroutines, workers, and processes when the driver supports lineage
- Composite runtime and worker paths route lineage members through the same backend lineage contract, so composite execution no longer bypasses overlap rules
- `CheckPresence` remains exact-key only; descendant membership markers are internal coordination state, not user-visible lock presence

## Migration Note

Applications that previously nested parent and child acquires across goroutines, workers, or processes may now receive `ErrOverlapRejected`.

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

## Phase 2a Example Guide

Use the examples below as the primary adoption path for Phase 2 and Phase 2a behavior.

### Start Here

- Single async worker lock with Redis lease plus idempotency:
  [`examples/phase2-basic/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-basic/README.md)
- Sync composite lock with canonical ordering on the memory driver:
  [`examples/phase2-composite-sync/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-composite-sync/README.md)
- Async composite worker claim with Redis:
  [`examples/phase2-composite-worker/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-composite-worker/README.md)
- Composite overlap rejection before callback execution:
  [`examples/phase2-overlap-reject/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-overlap-reject/README.md)
- Distributed parent-child rejection across managers and goroutines:
  [`examples/phase2-parent-child-runtime/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-parent-child-runtime/README.md)

### Which Example To Run

- Learn worker `ExecuteClaimed` with a single resource:
  `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/phase2-basic`
  Focus on `presence while held: held`, `idempotency after ack: completed`, and `duplicate outcome: ignored`.
- Learn sync composite execution:
  `go run ./examples/phase2-composite-sync`
  Focus on `composite acquired: account:acct-123,ledger:ledger-456` and `canonical order: ok`.
- Learn async composite worker execution:
  `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/phase2-composite-worker`
  Focus on `composite callback: ...` and `composite idempotency after ack: completed`.
- Learn reject-first overlap inside one composite request:
  `go run ./examples/phase2-overlap-reject`
  Focus on `overlap outcome: rejected`.
- Learn what Phase 2a added on top of Phase 2:
  `go run ./examples/phase2-parent-child-runtime`
  Focus on the two `overlap rejected` scenarios and the final Phase 2a note.

## Dependency Boundaries

- `go run ./examples/reentrant` shows nested acquire rejection is a reentrant guard, not dependency analysis.
- `go run ./examples/phase2-parent-child-runtime` shows current Phase 2a runtime behavior: parent-child overlap is rejected across managers.
- `go run ./examples/phase1-parent-child-metadata-only` is retained as a historical Phase 1 example, not the current Phase 2a behavior.

## Commands

- `go test ./...`
- `go test ./... -cover`
- `go run ./examples/basic`
- `go run ./examples/phase2-basic`
- `go run ./examples/phase2-composite-sync`
- `go run ./examples/phase2-composite-worker`
- `go run ./examples/phase2-overlap-reject`
- `go run ./examples/phase2-parent-child-runtime`
- `go run ./examples/contention`
- `go run ./examples/phase1-parent-child-metadata-only`
- `go run ./examples/reentrant`
- `go run ./examples/ttl`
