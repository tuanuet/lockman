# Lock Scenarios And Examples Expanded Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Regroup the lock scenarios guide around scenario families and add three new teaching examples that cover shared sync/async boundaries, parent-over-composite guidance, and shard ownership for bulk import.

**Architecture:** Treat this as one docs-and-examples increment. First restructure `docs/lock-scenarios-and-best-practices.md` so all scenarios, old and new, fit into explicit scenario families. Then add the three examples with deterministic output and local READMEs, and finally wire them back into the guide, README, and verification flow.

**Tech Stack:** Markdown documentation, Go examples, existing `runtime`, `workers`, Redis driver, Redis idempotency store, in-memory `testkit` driver

---

### Task 1: Regroup The Guide Around Scenario Families

**Files:**
- Modify: `docs/lock-scenarios-and-best-practices.md`
- Modify: `README.md`

- [ ] **Step 1: Write the failing structure check for the regrouped guide**

Run: `rg -n "^## |^### " docs/lock-scenarios-and-best-practices.md`
Expected: current output still shows the old flat `Real Scenarios` layout rather than explicit scenario families

- [ ] **Step 2: Replace the flat scenario section with explicit family headings**

Restructure `docs/lock-scenarios-and-best-practices.md` so the scenario area contains these family headings:

```md
## Scenario Families

### Single Aggregate Ownership
### Aggregate Versus Sub-Resource Concurrency
### Multi-Resource Coordination
### Sync And Async Shared Boundaries
### Lifecycle And Ownership Boundaries
### Shard Or Partition Ownership
### Advisory Visibility
### Migration And Compatibility
```

Keep the opening sections, pattern catalog, best practices, anti-patterns, decision matrix, and related-docs sections.

- [ ] **Step 3: Reassign all existing scenarios into the explicit families**

Move the existing scenarios under these exact family headings without losing their current structure:

- `Approve One Order` -> `### Single Aggregate Ownership`
- `Update One Order Item Under Order-Level Invariants` -> `### Aggregate Versus Sub-Resource Concurrency`
- `Transfer Between Two Accounts` -> `### Multi-Resource Coordination`
- `Inventory Reservation From A Queue Worker` -> `### Lifecycle And Ownership Boundaries`
- `Background Reconciliation Or Shard-Based Batch Job` -> `### Shard Or Partition Ownership`
- `Producer-Consumer Handoff` -> `### Lifecycle And Ownership Boundaries`
- `Admin Screen Or Operator Hint` -> `### Advisory Visibility`
- `Phase 2a Migration Scenario` -> `### Migration And Compatibility`
- `Shared Versus Split Sync/Async Definitions` -> `### Sync And Async Shared Boundaries`

Each scenario must still keep these exact subheadings:

```md
#### Problem
#### Recommended Pattern
#### Recommended Execution Package
#### Why This Choice
#### Example Key Shape
#### Best Practices
#### Common Mistakes
#### Architecture Note
```

- [ ] **Step 4: Add the three new scenarios into the right families**

Add these new scenarios:

- `Human Action And Background Worker Touch The Same Aggregate`
- `One Higher Aggregate Parent Lock Is Enough, Composite Is Overkill`
- `Bulk Import With Shard Ownership`

Required placement:

- `Human Action...` -> `### Sync And Async Shared Boundaries`
- `One Higher Aggregate Parent Lock...` -> `### Multi-Resource Coordination`
- `Bulk Import With Shard Ownership` -> `### Shard Or Partition Ownership`

Required content notes:

- `Human Action...` must recommend split sync and async definitions over the same aggregate key boundary as the default teaching case, while explaining when `ExecutionKind=both` could still be acceptable.
- `One Higher Aggregate Parent Lock...` must explicitly say composite is overkill because one higher aggregate parent lock already captures the invariant across multiple sub-resources in the same aggregate.
- `Bulk Import With Shard Ownership` must explain shard-level ownership as the default, compare it with smaller batch-level ownership, and keep the package recommendation in `workers`.

- [ ] **Step 5: Update the decision matrix and related links**

Expand the decision matrix to cover all twelve scenarios, but for the three new scenarios use temporary placeholders such as `new example to be added in Task 5` rather than concrete README links that do not exist yet.

Keep all existing docs/example links intact. Do not add concrete links to the three new example READMEs yet.

- [ ] **Step 6: Add the new guide link to the docs area in README if needed**

Keep the existing guide link in `README.md`, but make sure the surrounding wording still makes sense after the guide becomes a larger scenario handbook.

- [ ] **Step 7: Verify the regrouped guide structure**

Run: `rg -n "^## Scenario Families$|^### Single Aggregate Ownership$|^### Aggregate Versus Sub-Resource Concurrency$|^### Multi-Resource Coordination$|^### Sync And Async Shared Boundaries$|^### Lifecycle And Ownership Boundaries$|^### Shard Or Partition Ownership$|^### Advisory Visibility$|^### Migration And Compatibility$|^### Human Action And Background Worker Touch The Same Aggregate$|^### One Higher Aggregate Parent Lock Is Enough, Composite Is Overkill$|^### Bulk Import With Shard Ownership$" docs/lock-scenarios-and-best-practices.md`
Expected: matches for the scenario-families section, the eight family headings, and the three new scenario headings

- [ ] **Step 8: Verify the scenario subheading contract remains intact**

Run: `test "$(rg -c "^#### Problem$" docs/lock-scenarios-and-best-practices.md)" -eq 12 && test "$(rg -c "^#### Recommended Pattern$" docs/lock-scenarios-and-best-practices.md)" -eq 12 && test "$(rg -c "^#### Recommended Execution Package$" docs/lock-scenarios-and-best-practices.md)" -eq 12 && test "$(rg -c "^#### Why This Choice$" docs/lock-scenarios-and-best-practices.md)" -eq 12 && test "$(rg -c "^#### Example Key Shape$" docs/lock-scenarios-and-best-practices.md)" -eq 12 && test "$(rg -c "^#### Best Practices$" docs/lock-scenarios-and-best-practices.md)" -eq 12 && test "$(rg -c "^#### Common Mistakes$" docs/lock-scenarios-and-best-practices.md)" -eq 12 && test "$(rg -c "^#### Architecture Note$" docs/lock-scenarios-and-best-practices.md)" -eq 12 && echo ok`
Expected: `ok`

- [ ] **Step 9: Commit the regrouped guide**

```bash
git add README.md docs/lock-scenarios-and-best-practices.md
git commit -m "docs: regroup lock scenarios guide by family"
```

### Task 2: Add The Shared Aggregate Runtime/Worker Example

**Files:**
- Create: `examples/phase2-shared-aggregate-runtime-worker/main.go`
- Create: `examples/phase2-shared-aggregate-runtime-worker/main_test.go`
- Create: `examples/phase2-shared-aggregate-runtime-worker/README.md`

- [ ] **Step 1: Write the failing test for the new example output**

Create `examples/phase2-shared-aggregate-runtime-worker/main_test.go` with a Redis-gated test that expects output containing these lines:

```go
expected := []string{
    "runtime path: acquired order:123",
    "runtime definition: OrderApprovalSync",
    "worker path: claimed order:123",
    "worker definition: OrderApprovalAsync",
    "shared aggregate key: order:123",
    "teaching point: split sync and async definitions can still share one aggregate boundary",
    "shutdown: ok",
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./examples/phase2-shared-aggregate-runtime-worker -v`
Expected: FAIL because the example files do not exist yet

- [ ] **Step 3: Implement the example**

Create `main.go` that:

- builds a Redis client from `LOCKMAN_REDIS_URL`
- registers two definitions over the same key boundary:
  - one `sync` definition
  - one `async` definition
- uses `runtime` for the sync path and `workers` for the async path
- prints deterministic teaching output showing both the runtime path and the worker path, the shared aggregate key, and the split-definition recommendation
- shuts down cleanly

The implementation should not try to demonstrate a contrasting `ExecutionKind=both` runtime path. That contrast belongs in the README prose, not the executable flow.

- [ ] **Step 4: Add the local README**

Create `README.md` for this example that explains:

- the human-action plus background-worker scenario
- why the example uses split sync and async definitions on the same key boundary
- when `ExecutionKind=both` would still be acceptable
- the exact `go run` command

- [ ] **Step 5: Run the test to verify it passes**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./examples/phase2-shared-aggregate-runtime-worker -v`
Expected: PASS

- [ ] **Step 6: Commit the example**

```bash
git add examples/phase2-shared-aggregate-runtime-worker
git commit -m "feat(examples): add shared aggregate runtime worker example"
```

### Task 3: Add The Parent-Over-Composite Example

**Files:**
- Create: `examples/phase2-parent-over-composite/main.go`
- Create: `examples/phase2-parent-over-composite/main_test.go`
- Create: `examples/phase2-parent-over-composite/README.md`

- [ ] **Step 1: Write the failing test for the new example output**

Create `main_test.go` expecting:

```go
expected := []string{
    "aggregate lock: shipment:sh-123",
    "sub-resources involved: package-1,package-2",
    "teaching point: parent lock is enough, composite is overkill",
    "shutdown: ok",
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./examples/phase2-parent-over-composite -v`
Expected: FAIL because the example files do not exist yet

- [ ] **Step 3: Implement the example**

Create a memory-backed sync example showing:

- one aggregate parent lock
- multiple sub-resources inside the same business aggregate
- a narrative that composite would be over-modeling

Do not actually build a composite in this example. The point is to show why the parent boundary alone is enough.

- [ ] **Step 4: Add the local README**

Explain:

- the invariant being protected
- why the team might incorrectly reach for composite
- why one higher aggregate parent lock is enough here
- the exact `go run` command

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./examples/phase2-parent-over-composite -v`
Expected: PASS

- [ ] **Step 6: Commit the example**

```bash
git add examples/phase2-parent-over-composite
git commit -m "feat(examples): add parent over composite example"
```

### Task 4: Add The Bulk Import Shard Worker Example

**Files:**
- Create: `examples/phase2-bulk-import-shard-worker/main.go`
- Create: `examples/phase2-bulk-import-shard-worker/main_test.go`
- Create: `examples/phase2-bulk-import-shard-worker/README.md`

- [ ] **Step 1: Write the failing test for the new example output**

Create `main_test.go` with a Redis-gated output contract expecting:

```go
expected := []string{
    "shard lock: import-shard:07",
    "package: workers",
    "teaching point: shard ownership is the default boundary for bulk import",
    "contrast: smaller batch locks only work when batches are independently safe and replayable",
    "shutdown: ok",
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./examples/phase2-bulk-import-shard-worker -v`
Expected: FAIL because the example files do not exist yet

- [ ] **Step 3: Implement the example**

Create a Redis-backed worker example that:

- uses `workers`
- uses one shard or partition key as the claimed resource
- prints the shard-level ownership teaching points deterministically
- does not try to simulate a full import pipeline

- [ ] **Step 4: Add the local README**

Explain:

- why shard ownership is the default teaching case
- when smaller batch-level ownership would be reasonable
- the exact `go run` command

- [ ] **Step 5: Run the test to verify it passes**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./examples/phase2-bulk-import-shard-worker -v`
Expected: PASS

- [ ] **Step 6: Commit the example**

```bash
git add examples/phase2-bulk-import-shard-worker
git commit -m "feat(examples): add bulk import shard worker example"
```

### Task 5: Wire The New Examples Back Into The Guide

**Files:**
- Modify: `docs/lock-scenarios-and-best-practices.md`

- [ ] **Step 1: Add the three new example links into the relevant scenario sections**

Update the new scenarios so each one points readers to its matching example README in the relevant `Example Key Shape`, `Architecture Note`, or closing sentence.

- [ ] **Step 2: Add the three new example links into the decision matrix**

Update the matrix rows for the three new scenarios so `Next Doc/Example` points to:

- `examples/phase2-shared-aggregate-runtime-worker/README.md`
- `examples/phase2-parent-over-composite/README.md`
- `examples/phase2-bulk-import-shard-worker/README.md`

- [ ] **Step 3: Add the three new example links into the final related-docs/examples section**

Ensure the final related list contains all three new example READMEs.

- [ ] **Step 4: Verify the new links exist in the guide**

Run: `rg -n "phase2-shared-aggregate-runtime-worker/README|phase2-parent-over-composite/README|phase2-bulk-import-shard-worker/README" docs/lock-scenarios-and-best-practices.md && rg -n "examples/phase2-basic/README|examples/phase2-composite-sync/README|examples/phase2-composite-worker/README|examples/phase2-overlap-reject/README|examples/phase2-parent-child-runtime/README" docs/lock-scenarios-and-best-practices.md`
Expected: matches for all three new example links and all existing example links in the guide

- [ ] **Step 5: Commit the wiring changes**

```bash
git add docs/lock-scenarios-and-best-practices.md
git commit -m "docs: link new lock scenario examples"
```

### Task 6: Verify The Expanded Docs-And-Examples Increment End-To-End

**Files:**
- Verify: `docs/lock-scenarios-and-best-practices.md`
- Verify: `examples/phase2-shared-aggregate-runtime-worker/main.go`
- Verify: `examples/phase2-shared-aggregate-runtime-worker/main_test.go`
- Verify: `examples/phase2-shared-aggregate-runtime-worker/README.md`
- Verify: `examples/phase2-parent-over-composite/main.go`
- Verify: `examples/phase2-parent-over-composite/main_test.go`
- Verify: `examples/phase2-parent-over-composite/README.md`
- Verify: `examples/phase2-bulk-import-shard-worker/main.go`
- Verify: `examples/phase2-bulk-import-shard-worker/main_test.go`
- Verify: `examples/phase2-bulk-import-shard-worker/README.md`

- [ ] **Step 1: Verify the guide now contains all required family and scenario anchors**

Run: `rg -n "^## Scenario Families$|^### Single Aggregate Ownership$|^### Aggregate Versus Sub-Resource Concurrency$|^### Multi-Resource Coordination$|^### Sync And Async Shared Boundaries$|^### Lifecycle And Ownership Boundaries$|^### Shard Or Partition Ownership$|^### Advisory Visibility$|^### Migration And Compatibility$|^### Human Action And Background Worker Touch The Same Aggregate$|^### One Higher Aggregate Parent Lock Is Enough, Composite Is Overkill$|^### Bulk Import With Shard Ownership$" docs/lock-scenarios-and-best-practices.md`
Expected: all family and new-scenario anchors present

- [ ] **Step 2: Verify the new example directories and files exist**

Run: `test -f examples/phase2-shared-aggregate-runtime-worker/main.go && test -f examples/phase2-shared-aggregate-runtime-worker/main_test.go && test -f examples/phase2-shared-aggregate-runtime-worker/README.md && test -f examples/phase2-parent-over-composite/main.go && test -f examples/phase2-parent-over-composite/main_test.go && test -f examples/phase2-parent-over-composite/README.md && test -f examples/phase2-bulk-import-shard-worker/main.go && test -f examples/phase2-bulk-import-shard-worker/main_test.go && test -f examples/phase2-bulk-import-shard-worker/README.md && echo ok`
Expected: `ok`

- [ ] **Step 3: Run all example tests without Redis**

Run: `go test ./examples/... -v`
Expected: memory-backed examples PASS, Redis-backed examples PASS or SKIP when `LOCKMAN_REDIS_URL` is unset

- [ ] **Step 4: Run all example tests with Redis**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./examples/... -v`
Expected: PASS, including the three new Redis-backed examples

- [ ] **Step 5: Run the full repository test suite with Redis**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./...`
Expected: PASS

- [ ] **Step 6: Final commit if verification requires touch-ups**

```bash
git add README.md docs/lock-scenarios-and-best-practices.md examples/phase2-shared-aggregate-runtime-worker examples/phase2-parent-over-composite examples/phase2-bulk-import-shard-worker
git commit -m "docs: polish expanded lock scenarios guide"
```
