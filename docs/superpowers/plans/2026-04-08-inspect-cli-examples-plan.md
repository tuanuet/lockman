# Inspect CLI Examples Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create a demo server that generates realistic lock traffic and serves inspect endpoints, plus update the CLI README with quick-start scripts.

**Architecture:** A single `main.go` in `examples/inspect-server/` starts an HTTP server with the inspect handler and runs background goroutines that exercise sync Run, async Claim, and contention scenarios against an in-memory backend. The CLI README gets a Quick Start section with copy-paste commands.

**Tech Stack:** Go, lockman SDK, backend/memory, inspect, observe

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `examples/inspect-server/main.go` | Create | Demo server: HTTP + lock traffic |
| `cmd/inspect/README.md` | Modify | Add Quick Start section |

---

## Chunk 1: Demo Server

### Task 1: Create the demo server

**Files:**
- Create: `examples/inspect-server/main.go`

- [ ] **Step 1: Write the demo server**

```go
package main

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/backend/memory"
	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/observe"
)

type orderInput struct {
	OrderID string
}

var orderDef = lockman.DefineLock(
	"order",
	lockman.BindResourceID("order", func(in orderInput) string { return in.OrderID }),
)

var approveOrder = lockman.DefineRunOn("order.approve", orderDef)

// processOrder omits Idempotent() to avoid needing an idempotency store.
// This is intentional for the zero-infra demo — Claim works fine without it.
var processOrder = lockman.DefineClaimOn(
	"order.process",
	orderDef,
)

func main() {
	ctx := context.Background()

	port := os.Getenv("DEMO_PORT")
	if port == "" {
		port = ":8080"
	}

	store := inspect.NewStore()

	dispatcher := observe.NewDispatcher(
		observe.WithExporter(observe.ExporterFunc(func(_ context.Context, e observe.Event) error {
			return nil
		})),
	)

	reg := lockman.NewRegistry()
	if err := reg.Register(approveOrder, processOrder); err != nil {
		panic(err)
	}

	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "inspect-demo"}),
		lockman.WithBackend(memory.NewMemoryDriver()),
		lockman.WithObservability(lockman.Observability{
			Dispatcher: dispatcher,
			Store:      store,
		}),
	)
	if err != nil {
		panic(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", inspect.NewHandler(store))

	srv := &http.Server{Addr: port, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}()

	fmt.Println("Inspect demo server running on " + port)
	fmt.Println("TUI:      go run ./cmd/inspect")
	fmt.Println("Snapshot: go run ./cmd/inspect snapshot")
	fmt.Println("Events:   go run ./cmd/inspect events --kind contention")
	fmt.Println("Health:   go run ./cmd/inspect health")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go runSyncTraffic(ctx, client)
	go runAsyncTraffic(ctx, client)
	go runContentionTraffic(ctx, client)

	<-stop

	fmt.Println("\nshutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
	_ = client.Shutdown(shutdownCtx)
	fmt.Println("shutdown complete")
}

func runSyncTraffic(ctx context.Context, client *lockman.Client) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}

		orderID := fmt.Sprintf("ORD-%04d", rng.Intn(1000))
		req, err := approveOrder.With(orderInput{OrderID: orderID})
		if err != nil {
			continue
		}

		reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		_ = client.Run(reqCtx, req, func(_ context.Context, _ lockman.Lease) error {
			time.Sleep(time.Duration(rng.Intn(100)) * time.Millisecond)
			return nil
		})
		cancel()
	}
}

func runAsyncTraffic(ctx context.Context, client *lockman.Client) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	msgCounter := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}

		msgCounter++
		orderID := fmt.Sprintf("ORD-%04d", rng.Intn(1000))
		req, err := processOrder.With(orderInput{OrderID: orderID}, lockman.Delivery{
			MessageID:     fmt.Sprintf("msg-%d", msgCounter),
			ConsumerGroup: "demo-workers",
			Attempt:       1,
		})
		if err != nil {
			continue
		}

		reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_ = client.Claim(reqCtx, req, func(_ context.Context, _ lockman.Claim) error {
			time.Sleep(200 * time.Millisecond)
			return nil
		})
		cancel()
	}
}

func runContentionTraffic(ctx context.Context, client *lockman.Client) {
	const sharedOrderID = "ORD-CONTENTION"
	owners := []string{"owner-a", "owner-b"}

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}

		var wg sync.WaitGroup
		for _, owner := range owners {
			wg.Add(1)
			go func(owner string) {
				defer wg.Done()
				req, err := approveOrder.With(
					orderInput{OrderID: sharedOrderID},
					lockman.OwnerID(owner),
				)
				if err != nil {
					return
				}
				reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				_ = client.Run(reqCtx, req, func(_ context.Context, _ lockman.Lease) error {
					time.Sleep(1 * time.Second)
					return nil
				})
			}(owner)
		}
		wg.Wait()
	}
}
```

- [ ] **Step 2: Verify compilation**

Run: `go test -run '^$' ./examples/inspect-server/...`
Expected: PASS (compiles without errors)

> **TDD note:** This is a long-running demo process (not library code), so unit tests are intentionally skipped per the spec. Compilation verification serves as the test gate.

- [ ] **Step 3: Manual smoke test**

Run: `go run ./examples/inspect-server` (in one terminal)
Then: `go run ./cmd/inspect snapshot` (in another terminal)
Expected: JSON output with lock stats, no errors

- [ ] **Step 4: Commit**

```bash
git add examples/inspect-server/main.go
git commit -m "feat: add inspect demo server with traffic generation"
```

---

## Chunk 2: README Quick Start

### Task 2: Update CLI README

**Files:**
- Modify: `cmd/inspect/README.md`

- [ ] **Step 1: Read current README**

- [ ] **Step 2: Add Quick Start section and update Usage**

Prepend a Quick Start section and simplify the existing Usage block to reference the demo server default. The final README should look like:

```markdown
# lockman-inspect

Interactive TUI for inspecting lockman distributed locks.

## Quick Start

```bash
# Terminal 1: start the demo server (generates lock traffic)
go run ./examples/inspect-server

# Terminal 2: open the interactive TUI
go run ./cmd/inspect

# Or use one-shot commands (while demo server is running)
go run ./cmd/inspect snapshot
go run ./cmd/inspect events --kind contention
go run ./cmd/inspect health
```

## Usage

```bash
# Interactive TUI
lockman-inspect
lockman-inspect --url http://localhost:8080/locks/inspect

# One-shot commands
lockman-inspect snapshot --url ...
lockman-inspect active --url ...
lockman-inspect events --url ... --kind contention
lockman-inspect health --url ...
```

## Environment Variables

- `LOCKMAN_INSPECT_URL` — default base URL
- `DEMO_PORT` — override demo server port (default `:8080`)

## Screens

... (rest unchanged)
```

- [ ] **Step 3: Verify README renders correctly**

Run: `cat cmd/inspect/README.md`
Expected: Clean markdown, no broken formatting

- [ ] **Step 4: Commit**

```bash
git add cmd/inspect/README.md
git commit -m "docs: add quick start section to inspect CLI README"
```

---

## Chunk 3: Full Verification

### Task 3: Run full CI parity checks

- [ ] **Step 1: Run test suite**

Run: `go test ./...`
Expected: All existing tests pass + new example compiles

- [ ] **Step 2: Run workspace-off tests**

Run: `GOWORK=off go test ./...`
Expected: All pass

- [ ] **Step 3: Verify demo server + CLI end-to-end**

Run: `go run ./examples/inspect-server &` (background)
Wait 2s
Run: `go run ./cmd/inspect snapshot`
Run: `go run ./cmd/inspect active`
Run: `go run ./cmd/inspect events`
Run: `go run ./cmd/inspect health`
Expected: All return valid JSON, kill background server

- [ ] **Step 4: Commit if all green**

```bash
git add .
git commit -m "chore: verify inspect examples integration"
```
