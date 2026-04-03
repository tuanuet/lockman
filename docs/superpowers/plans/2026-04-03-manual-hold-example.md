# Manual Hold Example Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a dedicated SDK example for the `Hold` execution surface and surface it in the repository learning path.

**Architecture:** Follow the existing `examples/sdk` structure exactly: one tagged runnable `main.go` with a `run` helper, Redis wiring from environment, and a minimal hold acquire/forfeit flow. Keep the change documentation-focused by updating only the example indexes that already enumerate runnable examples.

**Tech Stack:** Go 1.22, root `lockman` SDK, Redis backend adapter, repository Markdown docs

---

## Chunk 1: Add the dedicated SDK example

### Task 1: Create the example entrypoint

**Files:**
- Create: `examples/sdk/manual-hold/main.go`
- Reference: `examples/sdk/sync-approve-order/main.go`
- Reference: `examples/sdk/shared-lock-definition/main.go`

- [ ] **Step 1: Write the failing compile target mentally and mirror existing example shape**

The new file should compile under the `lockman_examples` build tag and expose:

```go
func main()
func run(out io.Writer, redisClient goredis.UniversalClient) error
func redisClientFromEnv() (*goredis.Client, error)
```

- [ ] **Step 2: Implement the minimal example**

Use this structure:

```go
type holdInput struct {
	OrderID string
}

var orderDef = lockman.DefineLock(
	"order",
	lockman.BindResourceID("order", func(in holdInput) string { return in.OrderID }),
)

var manualHold = lockman.DefineHoldOn("order.manual_hold", orderDef)
```

The `run(...)` function should:

```go
reg := lockman.NewRegistry()
if err := reg.Register(manualHold); err != nil { return err }

client, err := lockman.New(
	lockman.WithRegistry(reg),
	lockman.WithIdentity(lockman.Identity{OwnerID: "orders-api"}),
	lockman.WithBackend(lockredis.New(redisClient, "")),
)
if err != nil { return err }
defer client.Shutdown(context.Background())

req, err := manualHold.With(holdInput{OrderID: "123"})
if err != nil { return err }

handle, err := client.Hold(context.Background(), req)
if err != nil { return err }

fmt.Fprintf(out, "hold resource key: %s\n", req.ResourceKey())
fmt.Fprintf(out, "hold token: %s\n", handle.Token())

if err := client.Forfeit(context.Background(), manualHold.ForfeitWith(handle.Token())); err != nil {
	return err
}

fmt.Fprintln(out, "forfeit: ok")
fmt.Fprintln(out, "shutdown: ok")
```

- [ ] **Step 3: Keep naming and messaging aligned with existing examples**

Requirements:
- same `example failed: ...` stderr pattern in `main()`
- same Redis URL loading behavior
- no extra concepts like `Run`, `Claim`, `ErrBusy`, strict, or composite

## Chunk 2: Make the example discoverable

### Task 2: Update README learning path

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add the new example to the learning-path list**

Insert a new bullet in the execution-surface section near the existing sync and async examples:

```md
- [`examples/sdk/manual-hold`](examples/sdk/manual-hold): the shortest manual hold acquire/forfeit flow on the SDK path
```

- [ ] **Step 2: Keep ordering intuitive**

Recommended order:
- sync run
- manual hold
- async claim

This keeps all three execution surfaces visible together.

### Task 3: Update production-oriented docs if they enumerate runnable examples

**Files:**
- Modify: `docs/production-guide.md`

- [ ] **Step 1: Check whether the guide has a short list of copyable examples**

If it already lists example directories, add the hold example in the same style. If no such list exists or the addition would be awkward, keep this change minimal and skip unnecessary wording churn.

## Chunk 3: Verify the touched areas

### Task 4: Compile the examples

**Files:**
- Test scope: `examples/...`

- [ ] **Step 1: Run the tagged examples compile check**

Run: `go test -tags lockman_examples ./examples/... -run '^$'`
Expected: PASS

- [ ] **Step 2: Run a focused root compile check if needed**

Run: `go test ./... -run '^$'`
Expected: PASS or, if unrelated workspace issues exist, capture the exact failing package and confirm it is unrelated to the touched files.

- [ ] **Step 3: Review changed files for scope control**

Expected changed files:
- `examples/sdk/manual-hold/main.go`
- `README.md`
- optionally `docs/production-guide.md`

No unrelated refactors.
