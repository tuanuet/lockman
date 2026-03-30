# Lock Management Platform Phase 2 Examples Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add focused Phase 2 examples that demonstrate worker, composite, and overlap-rejection behavior with deterministic output and matching tests.

**Architecture:** Keep each example isolated around one Phase 2 concept. Memory-backed examples should exercise sync composite execution and reject-first overlap behavior without external services, while Redis-backed examples should demonstrate worker execution and idempotency behavior against the production Phase 2 backend surfaces already implemented in the repo.

**Tech Stack:** Go 1.22+, standard library, `testing` package, existing `lockkit` runtime/workers/registry/testkit` packages, Redis via `github.com/redis/go-redis/v9`

---

## Planned File Structure

### Existing files to extend

- `README.md`: list Phase 2 example commands and explain which ones require Redis
- `examples/async-single-resource/main.go`: keep current single-resource worker example unchanged unless output/docs need alignment
- `examples/async-single-resource/main_test.go`: keep current output contract test unchanged unless output/docs need alignment

### New files to add

- `examples/sync-composite-lock/main.go`: sync composite example using `runtime.NewManager` and the memory driver
- `examples/sync-composite-lock/main_test.go`: output-contract test for the sync composite example
- `examples/async-composite-lock/main.go`: async composite worker example using Redis driver and Redis idempotency store
- `examples/async-composite-lock/main_test.go`: Redis-gated output-contract test for the composite worker example
- `examples/composite-overlap-reject/main.go`: overlap rejection example using a parent-child composite on the memory driver
- `examples/composite-overlap-reject/main_test.go`: output-contract test proving overlap rejection is surfaced before callback execution

## Scope

This plan adds only example coverage and related documentation for already-implemented Phase 2 behavior:

- single-resource async worker flow via the existing `phase2-basic` example
- sync standard-mode composite execution
- async standard-mode composite worker execution
- reject-first parent-child overlap behavior

It does **not** change:

- Phase 2 public API
- runtime semantics
- worker outcome mapping rules
- strict-mode support
- Redis driver/idempotency implementation details outside what examples already consume

## Task 1: Add Sync Composite Example

**Files:**
- Create: `examples/sync-composite-lock/main.go`
- Test: `examples/sync-composite-lock/main_test.go`

- [ ] **Step 1: Write the failing example test**

```go
func TestRunPrintsCompositeSyncFlow(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	expected := []string{
		"composite acquired: account:acct-123,ledger:ledger-456",
		"canonical order: ok",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./examples/sync-composite-lock -v`
Expected: FAIL because `main.go` and `run` do not exist yet

- [ ] **Step 3: Write the minimal sync composite example**

```go
reg := registry.New()
// Register AccountMember, LedgerMember, and TransferComposite.

mgr, err := runtime.NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
if err != nil {
	return err
}

err = mgr.ExecuteCompositeExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
	joined := strings.Join(lease.ResourceKeys, ",")
	if _, err := fmt.Fprintf(out, "composite acquired: %s\n", joined); err != nil {
		return err
	}
	if joined == "account:acct-123,ledger:ledger-456" {
		_, err := fmt.Fprintln(out, "canonical order: ok")
		return err
	}
	return fmt.Errorf("unexpected canonical order: %s", joined)
})
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./examples/sync-composite-lock -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add examples/sync-composite-lock/main.go examples/sync-composite-lock/main_test.go
git commit -m "feat(examples): add phase 2 composite sync example"
```

## Task 2: Add Composite Worker Example

**Files:**
- Create: `examples/async-composite-lock/main.go`
- Test: `examples/async-composite-lock/main_test.go`

- [ ] **Step 1: Write the failing Redis-gated example test**

```go
func TestRunPrintsCompositeWorkerFlow(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("LOCKMAN_REDIS_URL"))
	if redisURL == "" {
		t.Skip("LOCKMAN_REDIS_URL is not set")
	}

	var out bytes.Buffer
	if err := run(&out, redisURL); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	expected := []string{
		"composite callback: account:acct-123,ledger:ledger-456",
		"composite idempotency after ack: completed",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./examples/async-composite-lock -v`
Expected: FAIL because `main.go` and `run` do not exist yet, or SKIP when `LOCKMAN_REDIS_URL` is unset

- [ ] **Step 3: Write the minimal composite worker example**

```go
client, err := newRedisClient(redisURL)
if err != nil {
	return err
}

driver := redisdriver.NewDriver(client, prefix+":lease")
store := redisstore.NewStore(client, prefix+":idempotency")
mgr, err := workers.NewManager(reg, driver, store)
if err != nil {
	return err
}

err = mgr.ExecuteCompositeClaimed(context.Background(), req, func(ctx context.Context, claim definitions.ClaimContext) error {
	_, err := fmt.Fprintf(out, "composite callback: %s\n", strings.Join(claim.ResourceKeys, ","))
	return err
})
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./examples/async-composite-lock -v`
Expected: PASS when `LOCKMAN_REDIS_URL` is set, otherwise SKIP

- [ ] **Step 5: Commit**

```bash
git add examples/async-composite-lock/main.go examples/async-composite-lock/main_test.go
git commit -m "feat(examples): add phase 2 composite worker example"
```

## Task 3: Add Overlap Rejection Example

**Files:**
- Create: `examples/composite-overlap-reject/main.go`
- Test: `examples/composite-overlap-reject/main_test.go`

- [ ] **Step 1: Write the failing example test**

```go
func TestRunPrintsOverlapRejectFlow(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	expected := []string{
		"overlap outcome: rejected",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./examples/composite-overlap-reject -v`
Expected: FAIL because `main.go` and `run` do not exist yet

- [ ] **Step 3: Write the minimal overlap rejection example**

```go
err = mgr.ExecuteCompositeExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
	return errors.New("callback should not run")
})
switch {
case errors.Is(err, lockerrors.ErrPolicyViolation):
	_, err = fmt.Fprintln(out, "overlap outcome: rejected")
case err != nil:
	return err
default:
	return fmt.Errorf("expected overlap rejection")
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./examples/composite-overlap-reject -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add examples/composite-overlap-reject/main.go examples/composite-overlap-reject/main_test.go
git commit -m "feat(examples): add phase 2 overlap rejection example"
```

## Task 4: Update README Example Commands

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Write the failing documentation expectation in the README manually**

Add a Phase 2 examples section that lists:

- `go run ./examples/async-single-resource`
- `go run ./examples/sync-composite-lock`
- `go run ./examples/async-composite-lock`
- `go run ./examples/composite-overlap-reject`

And distinguish Redis-backed examples from memory-backed examples.

- [ ] **Step 2: Run a grep check to verify the new commands are not documented yet**

Run: `rg -n "phase2-composite-sync|phase2-composite-worker|phase2-overlap-reject" README.md`
Expected: no matches

- [ ] **Step 3: Update README with the minimal Phase 2 example documentation**

```md
## Phase 2 Examples

Redis-backed:

- `go run ./examples/async-single-resource`
- `go run ./examples/async-composite-lock`

Memory-backed:

- `go run ./examples/sync-composite-lock`
- `go run ./examples/composite-overlap-reject`
```

- [ ] **Step 4: Run the grep check to verify the new commands are documented**

Run: `rg -n "phase2-composite-sync|phase2-composite-worker|phase2-overlap-reject" README.md`
Expected: matching lines in the new Phase 2 examples section

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: add phase 2 example commands"
```

## Task 5: Verify Example Coverage End-To-End

**Files:**
- Verify: `examples/async-single-resource/main.go`
- Verify: `examples/async-single-resource/main_test.go`
- Verify: `examples/sync-composite-lock/main.go`
- Verify: `examples/sync-composite-lock/main_test.go`
- Verify: `examples/async-composite-lock/main.go`
- Verify: `examples/async-composite-lock/main_test.go`
- Verify: `examples/composite-overlap-reject/main.go`
- Verify: `examples/composite-overlap-reject/main_test.go`
- Verify: `README.md`

- [ ] **Step 1: Run all example tests**

Run: `go test ./examples/... -v`
Expected: PASS for memory-backed examples, PASS or SKIP for Redis-backed examples depending on `LOCKMAN_REDIS_URL`

- [ ] **Step 2: Run the full repository test suite**

Run: `go test ./...`
Expected: PASS, with Redis integration tests and Redis-backed examples skipped when `LOCKMAN_REDIS_URL` is unset

- [ ] **Step 3: Run the memory-backed examples directly**

Run:

```bash
go run ./examples/sync-composite-lock
go run ./examples/composite-overlap-reject
```

Expected: the documented output contracts print successfully

- [ ] **Step 4: Run the Redis-backed examples directly when Redis is available**

Run:

```bash
go run ./examples/async-single-resource
go run ./examples/async-composite-lock
```

Expected: the documented output contracts print successfully when `LOCKMAN_REDIS_URL` points at a reachable Redis instance

- [ ] **Step 5: Commit**

```bash
git add README.md examples/async-single-resource examples/sync-composite-lock examples/async-composite-lock examples/composite-overlap-reject
git commit -m "test(examples): verify phase 2 example coverage"
```
