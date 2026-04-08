# Inspect CLI Examples Design

## Goal

Provide a runnable demo server and updated README scripts so users can immediately test the `lockman-inspect` TUI without setting up their own application.

## Deliverables

### 1. Demo Server — `examples/inspect-server/`

**Purpose:** A long-running process that generates realistic lock traffic and serves inspect HTTP endpoints on `localhost:8080`.

**Structure:**
- `examples/inspect-server/main.go` — single-file server
- No `main_test.go` — this is a long-running demo process, not a runnable scenario. Compilation is verified by `go test -run '^$' ./examples/inspect-server/...`. Unlike `examples/sdk/` examples which demonstrate API usage patterns, this exists purely to feed data to the CLI TUI.

**Behavior:**

1. Creates a `lockman` client with:
   - `inspect.NewStore()` for local state
   - `observe.NewDispatcher()` with a no-op exporter (events flow into the store, stdout is optional)
   - In-memory backend (`backend/memory`) — no Redis dependency
   - Identity: `"inspect-demo"`
   - Observability: `lockman.WithObservability(lockman.Observability{Dispatcher: dispatcher, Store: store})`
   - Registry with 2 use cases: `order.approve` (sync `Run`) and `order.process` (async `Claim` **without** idempotency — kept simple for zero-infra demo)

2. Mounts inspect handler as a subtree at `/locks/` (effective base path: `/locks/inspect`) on `http://:8080`

3. Starts 3 background goroutines generating traffic:
   - **Sync loop** — calls `approveOrder` for random order IDs every 500ms
   - **Async loop** — claims `processOrder` every 2s (no idempotency, plain `Claim`) with brief renewal sleep
   - **Contention loop** — 2 owners race on the same resource every 5s

4. Prints startup instructions with configurable port (`DEMO_PORT` env var, default `:8080`):
   ```
   Inspect demo server running on :8080
   TUI: go run ./cmd/inspect
   Snapshot: go run ./cmd/inspect snapshot
   Events: go run ./cmd/inspect events --kind contention
   Health: go run ./cmd/inspect health
   ```

5. Runs until `Ctrl+C`, then graceful shutdown: `dispatcher.Shutdown(ctx)` then `client.Shutdown(ctx)`

**Key decisions:**
- Uses in-memory backend so no Docker/Redis needed
- No build tag — `go run ./examples/inspect-server` works directly
- Traffic patterns cover all 5 TUI screens meaningfully

### 2. README Update — `cmd/inspect/README.md`

Add a **Quick Start** section at the top with copy-paste commands:

```bash
# Terminal 1: start the demo server (generates lock traffic)
go run ./examples/inspect-server

# Terminal 2: open the interactive TUI
go run ./cmd/inspect

# Or use one-shot commands
go run ./cmd/inspect snapshot
go run ./cmd/inspect events --kind contention
go run ./cmd/inspect health
```

Replace the existing Usage section's hardcoded URL with the default (no `--url` needed when demo server runs).

## Dependencies

- `backend/memory` — already exists, used for zero-infra demo
- No new module or `go.mod` — added to workspace like other examples
- No build tag (unlike `examples/sdk/` which need `lockman_examples`)

## Testing

- Manual verification: `go run ./examples/inspect-server` + `go run ./cmd/inspect`
- CI compile check: `go test -run '^$' ./examples/inspect-server/...`
