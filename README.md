# lockman

Distributed lock platform SDK prototype for Go.

## Phase 1 Status

- Standard-mode exclusive execution via `ExecuteExclusive`
- Advisory presence checks via `CheckPresence`
- Lifecycle shutdown via `Shutdown(ctx)`
- Central registry validation plus the in-memory `testkit` driver
- Parent-lock focused scope with baseline runtime metrics

## Commands

- `go test ./...`
- `go test ./... -cover`
