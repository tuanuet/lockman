# backend/memory Public Testkit — Design Spec

> **Status:** Draft
> **Date:** 2026-04-05
> **Author:** lockman team

## Problem

Users who want to unit-test code that uses lockman currently need Redis (via `miniredis`) or must import an internal package (`lockkit/testkit`). The internal package lives under `lockkit/`, which signals "not for public consumption" and forces users to depend on internal implementation details.

## Goal

Provide a clean, public, importable in-memory backend at `backend/memory` so users can write unit tests without Redis or external dependencies.

## Scope

### In scope

- Move `lockkit/testkit/memory_driver.go` → `backend/memory/driver.go`
- Move `lockkit/testkit/memory_driver_test.go` → `backend/memory/driver_test.go`
- Move `lockkit/testkit/assertions.go` → `backend/memory/assertions.go`
- Update all internal imports (~30 files across root, lockkit, benchmarks, examples, advanced)
- Delete `lockkit/testkit/` directory
- Update documentation references (`SKILL.md`)

### Out of scope

- Adding new features to MemoryDriver
- Backward compatibility / re-export from old location
- Separate `go.mod` for `backend/memory` (no external deps needed)
- Changes to `backend/redis` or other backends

## Design

### Package location

`backend/memory` — sibling to `backend/redis`, under the root module.

```
backend/
  contracts.go
  contracts_test.go
  redis/
  memory/
    driver.go          # MemoryDriver implementation
    driver_test.go     # Internal tests
    assertions.go      # AssertSingleResourceLease helper
```

### Interfaces implemented

`MemoryDriver` implements:
- `backend.Driver` (Acquire, Renew, Release, CheckPresence, Ping)
- `backend.StrictDriver` (AcquireStrict, RenewStrict, ReleaseStrict)
- `backend.LineageDriver` (AcquireWithLineage, RenewWithLineage, ReleaseWithLineage)

No `ForceReleaseDriver` — not needed for testing.

### Public API surface

```go
package memory

func NewMemoryDriver() *MemoryDriver
func AssertSingleResourceLease(t *testing.T, lease backend.LeaseRecord, defID, ownerID, resourceKey string)
```

That's it. Users create a driver, pass it to `lockman.New(registry, driver)`, and test.

### Key changes from original

1. **Package name:** `testkit` → `memory`
2. **Error reference:** `lockerrors.ErrOverlapRejected` → `backend.ErrOverlapRejected` (2 occurrences in `rejectLineageConflict`)
3. **Import path:** `github.com/tuanuet/lockman/lockkit/testkit` → `github.com/tuanuet/lockman/backend/memory`
4. **No dependency on `lockkit/errors`** — uses `backend.ErrOverlapRejected` directly (they are the same sentinel)

### Usage example

```go
func TestOrderProcessing(t *testing.T) {
    driver := memory.NewMemoryDriver()
    registry := lockman.NewRegistry()
    registry.DefineRun("order.lock", lockman.RunConfig{
        Backend: driver,
    })

    client := lockman.New(registry, driver)

    err := client.Run(context.Background(), "order.lock", "order:123", func(ctx context.Context) error {
        // test logic
        return nil
    })
    if err != nil {
        t.Fatal(err)
    }
}
```

## Migration impact

| Area | Impact |
|------|--------|
| Root tests | Import path change only |
| `lockkit/*` tests | Import path change only |
| `benchmarks/` | Import path change only |
| `examples/` | Import path change only |
| `advanced/` | Import path change only |
| `SKILL.md` | Doc reference update |
| `docs/superpowers/plans/` | Historical references remain (no change needed — they document past work) |

## Verification

- `go test ./backend/memory -v` — all memory driver tests pass
- `go test ./...` — full suite passes
- `GOWORK=off go test ./...` — workspace-off suite passes
- `go test -tags lockman_examples ./examples/... -run '^$'` — examples compile
- `make lint` — no lint issues
- `grep -r 'lockkit/testkit' --include='*.go' .` — zero results
