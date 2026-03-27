# Lock Scenarios And Best Practices Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a new scenario-driven guidance document that helps application teams and architects choose the right lock pattern for real product flows without duplicating the existing runtime/workers or definition reference docs.

**Architecture:** Keep the implementation documentation-only. Build one new guide at `docs/lock-scenarios-and-best-practices.md`, then wire it into the repo navigation from `README.md`. The guide should stay pattern-and-scenario driven, link out to existing reference docs where details already exist, and ground recommendations in the Phase 2a examples already present in the repo.

**Tech Stack:** Markdown documentation, existing docs under `docs/`, existing example READMEs under `examples/`

---

### Task 1: Create The New Guide Skeleton And Navigation Entry

**Files:**
- Create: `docs/lock-scenarios-and-best-practices.md`
- Modify: `README.md`

- [ ] **Step 1: Write the initial document skeleton in the new guide**

Create `docs/lock-scenarios-and-best-practices.md` with these top-level headings only:

```md
# Lock Scenarios And Best Practices

## Who This Guide Is For

## Decision Framing

## Quick Decision Guide

## Pattern Catalog

## Real Scenarios

## Best Practices

## Anti-Patterns

## Decision Matrix

## Related Docs And Examples
```

- [ ] **Step 2: Verify the skeleton exists and headings are present**

Run: `rg -n "^#|^##" docs/lock-scenarios-and-best-practices.md`
Expected: one `#` heading and the nine `##` headings above

- [ ] **Step 3: Write the opening audience and purpose sections**

Under `## Who This Guide Is For`, add:

- one short paragraph explaining what problem this guide solves
- one subsection or paragraph for application engineers
- one subsection or paragraph for architects and tech leads

This opening must explain that the guide is scenario-driven and governance-driven, not an API reference.

- [ ] **Step 4: Verify the opening content exists**

Run: `rg -n "application engineers|architects and tech leads|scenario-driven|governance-driven|not an API reference" docs/lock-scenarios-and-best-practices.md`
Expected: matches for all five phrases in the new guide

- [ ] **Step 5: Add the README navigation entry**

Modify `README.md` to add a short bullet under the `## Phase 2 Status` list, next to the other documentation links, linking to:

- `docs/lock-scenarios-and-best-practices.md`

The README text should position it as the scenario and best-practices guide, not as an API reference.

- [ ] **Step 6: Verify the README link was added**

Run: `rg -n "lock-scenarios-and-best-practices" README.md`
Expected: one new README line linking to the new guide

- [ ] **Step 7: Commit the scaffold**

```bash
git add README.md docs/lock-scenarios-and-best-practices.md
git commit -m "docs: scaffold lock scenarios guide"
```

### Task 2: Write Decision Framing, Quick Guide, And Pattern Catalog

**Files:**
- Modify: `docs/lock-scenarios-and-best-practices.md`

- [ ] **Step 1: Write the failing structural check for the early sections**

Verify that the decision-framing terms and pattern-body subheadings do not yet exist:

Run: `rg -n "ordinary lock contention|parent-child overlap rejection|same-process reentrant rejection|duplicate-delivery|advisory presence|#### What It Is|#### Good Fit|#### Bad Fit|#### Trade-Offs|#### Architecture And Registry Notes" docs/lock-scenarios-and-best-practices.md`
Expected: no matches yet

- [ ] **Step 2: Write the Decision Framing section**

Add short explanations that distinguish:

- ordinary lock contention
- parent-child overlap rejection
- same-process reentrant rejection
- duplicate-delivery/idempotency concerns
- advisory presence checks versus correctness guarantees

Keep each distinction brief and use plain language.

- [ ] **Step 3: Write the Quick Decision Guide section**

Add a concise decision list covering:

- direct caller waiting now -> `runtime`
- replayable async delivery -> `workers`
- one aggregate invariant -> parent lock
- independent sub-resource concurrency -> child lock
- multiple independent resources together -> composite
- operator hint only -> presence check only
- correctness depending on presence -> redesign

Add a short link-out to `docs/runtime-vs-workers.md` instead of re-explaining that full topic.

- [ ] **Step 4: Write the Pattern Catalog section**

Add subsections for:

- parent lock
- child lock
- composite lock
- presence-check-only usage

Use these exact per-pattern subheadings:

```md
#### What It Is
#### Good Fit
#### Bad Fit
#### Trade-Offs
#### Architecture And Registry Notes
```

Do not add `runtime` or `workers` as standalone pattern entries.

- [ ] **Step 5: Verify the early sections contain all required terms**

Run: `test "$(rg -c "^#### What It Is$" docs/lock-scenarios-and-best-practices.md)" -eq 4 && test "$(rg -c "^#### Good Fit$" docs/lock-scenarios-and-best-practices.md)" -eq 4 && test "$(rg -c "^#### Bad Fit$" docs/lock-scenarios-and-best-practices.md)" -eq 4 && test "$(rg -c "^#### Trade-Offs$" docs/lock-scenarios-and-best-practices.md)" -eq 4 && test "$(rg -c "^#### Architecture And Registry Notes$" docs/lock-scenarios-and-best-practices.md)" -eq 4 && echo ok`
Expected: `ok`

- [ ] **Step 6: Commit the early sections**

```bash
git add docs/lock-scenarios-and-best-practices.md
git commit -m "docs: add lock pattern decision guide"
```

### Task 3: Write The Real Scenarios Section

**Files:**
- Modify: `docs/lock-scenarios-and-best-practices.md`

- [ ] **Step 1: Write the scenario heading checklist**

Use these exact scenario headings:

```md
### Approve One Order
### Update One Order Item Under Order-Level Invariants
### Transfer Between Two Accounts
### Inventory Reservation From A Queue Worker
### Background Reconciliation Or Shard-Based Batch Job
### Producer-Consumer Handoff
### Admin Screen Or Operator Hint
### Phase 2a Migration Scenario
### Shared Versus Split Sync/Async Definitions
```

- [ ] **Step 2: Verify the scenario headings are absent before writing**

Run: `rg -n "^### " docs/lock-scenarios-and-best-practices.md`
Expected: no scenario headings yet, or fewer than the nine required headings

- [ ] **Step 3: Write all nine scenarios**

For each scenario, include all of:

Use these exact per-scenario subheadings:

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

Additional scenario-specific requirements:

- `Approve One Order`: use one parent lock and `runtime`
- `Update One Order Item Under Order-Level Invariants`: explain `child with lineage` versus `parent-only is simpler`, and mention Phase 2a overlap behavior
- `Transfer Between Two Accounts`: explicitly state that `two manual acquires are inferior`
- `Inventory Reservation From A Queue Worker`: explicitly state that `async delivery semantics matter as much as locking`
- `Background Reconciliation Or Shard-Based Batch Job`: state the default example is queue-triggered, prefer shard lock when the invariant is one worker per shard, prefer per-batch lock only when batches are independently safe and replayable
- `Producer-Consumer Handoff`: explicitly reject `lock in producer, release in consumer` and state that `claim ownership begins in the consumer`
- `Admin Screen Or Operator Hint`: explain that `CheckPresence` is advisory only and `must not gate correctness-critical writes`
- `Phase 2a Migration Scenario`: explain both parent-held-child and child-held-parent rejection after Phase 2a, and explicitly say `previously permissive flows are now rejected`
- `Shared Versus Split Sync/Async Definitions`: define when `ExecutionKind=both` is acceptable, when separate definitions are safer, and include `registry review` guidance

- [ ] **Step 4: Verify the scenario section is structurally complete**

Run: `test "$(rg -c "^### " docs/lock-scenarios-and-best-practices.md)" -ge 13 && test "$(rg -c "^#### Problem$" docs/lock-scenarios-and-best-practices.md)" -eq 9 && test "$(rg -c "^#### Recommended Pattern$" docs/lock-scenarios-and-best-practices.md)" -eq 9 && test "$(rg -c "^#### Recommended Execution Package$" docs/lock-scenarios-and-best-practices.md)" -eq 9 && test "$(rg -c "^#### Why This Choice$" docs/lock-scenarios-and-best-practices.md)" -eq 9 && test "$(rg -c "^#### Example Key Shape$" docs/lock-scenarios-and-best-practices.md)" -eq 9 && test "$(rg -c "^#### Best Practices$" docs/lock-scenarios-and-best-practices.md)" -eq 9 && test "$(rg -c "^#### Common Mistakes$" docs/lock-scenarios-and-best-practices.md)" -eq 9 && test "$(rg -c "^#### Architecture Note$" docs/lock-scenarios-and-best-practices.md)" -eq 9 && rg -n "one parent lock|child with lineage|parent-only is simpler|two manual acquires are inferior|async delivery semantics matter as much as locking|one worker may process this shard at a time|independently safe and replayable|lock in producer, release in consumer|claim ownership begins in the consumer|CheckPresence|must not gate correctness-critical writes|parent-held child|child-held parent|previously permissive flows are now rejected|ExecutionKind=both|split definitions|registry review" docs/lock-scenarios-and-best-practices.md && echo ok`
Expected: `ok`

- [ ] **Step 5: Commit the scenario section**

```bash
git add docs/lock-scenarios-and-best-practices.md
git commit -m "docs: add real-world lock scenarios"
```

### Task 4: Add Best Practices, Anti-Patterns, Decision Matrix, And Cross-Links

**Files:**
- Modify: `docs/lock-scenarios-and-best-practices.md`
- Modify: `README.md`

- [ ] **Step 1: Write the Best Practices section as governance heuristics**

Add concise, opinionated guidance for:

- central registry ownership and review
- business-readable definition IDs
- template key builders and explicit fields
- TTL sizing guidance
- keeping composites small
- avoiding nested manual orchestration
- preferring parent locks when aggregate invariants dominate
- using child locks only when intentional sub-resource concurrency exists
- treating `CheckPresence` as advisory only
- aligning async idempotency with message ownership
- distinguishing overlap rejection from lock busy
- choosing `ExecutionKind=both` versus split definitions

When a point depends on field semantics, link to `docs/lock-definition-reference.md` rather than rewriting its full explanation.

- [ ] **Step 2: Write the Anti-Patterns section**

Cover each required anti-pattern with:

- why it is risky
- what to do instead

Required anti-patterns:

- too many children where one parent is simpler
- one coarse parent where sub-resource concurrency is required
- composite where one parent lock is enough
- manual multi-lock orchestration in app code
- producer-acquire and consumer-release
- presence as correctness signal
- one definition reused across unrelated semantics
- pre-Phase-2a assumption that parent-child overlap is metadata only

- [ ] **Step 3: Write the Decision Matrix**

Add a compact Markdown table with columns:

- scenario type
- recommended lock shape
- package
- why
- next doc/example

Use all nine required scenarios as the row set.

- [ ] **Step 4: Write the Related Docs And Examples section**

Link explicitly to:

- `docs/runtime-vs-workers.md`
- `docs/lock-definition-reference.md`
- `examples/phase2-basic/README.md`
- `examples/phase2-composite-sync/README.md`
- `examples/phase2-composite-worker/README.md`
- `examples/phase2-overlap-reject/README.md`
- `examples/phase2-parent-child-runtime/README.md`

- [ ] **Step 5: Verify all required cross-links exist**

Run: `rg -n "runtime-vs-workers|lock-definition-reference|phase2-basic/README|phase2-composite-sync/README|phase2-composite-worker/README|phase2-overlap-reject/README|phase2-parent-child-runtime/README" docs/lock-scenarios-and-best-practices.md`
Expected: matches for all required docs and example links in the new guide itself

- [ ] **Step 6: Commit the finished guide**

```bash
git add README.md docs/lock-scenarios-and-best-practices.md
git commit -m "docs: add lock scenarios and best practices guide"
```

### Task 5: Verify The Documentation End-To-End

**Files:**
- Verify: `README.md`
- Verify: `docs/lock-scenarios-and-best-practices.md`

- [ ] **Step 1: Run structural verification against the acceptance criteria**

Run: `rg -n "^## |^### |Decision Matrix|Anti-Patterns|Best Practices|Related Docs And Examples|Phase 2a Migration Scenario|Shared Versus Split Sync/Async Definitions|application engineers|architects and tech leads" docs/lock-scenarios-and-best-practices.md`
Expected: all required major sections, opening audience phrases, and scenario headings are present

- [ ] **Step 2: Verify the guide avoids silent duplication of the existing docs**

Run: `rg -n "use runtime for synchronous|use workers for asynchronous|Field reference|type LockDefinition struct" docs/lock-scenarios-and-best-practices.md`
Expected: no verbatim copy of the existing runtime/workers short version or definition reference code block

- [ ] **Step 3: Verify links and examples still exist**

Run: `test -f docs/runtime-vs-workers.md && test -f docs/lock-definition-reference.md && test -f examples/phase2-basic/README.md && test -f examples/phase2-composite-sync/README.md && test -f examples/phase2-composite-worker/README.md && test -f examples/phase2-overlap-reject/README.md && test -f examples/phase2-parent-child-runtime/README.md && echo ok`
Expected: `ok`

- [ ] **Step 4: Verify repository example coverage still passes**

Run: `go test ./examples/... -v`
Expected: PASS for memory-backed examples, PASS or SKIP for Redis-backed examples when `LOCKMAN_REDIS_URL` is unset

- [ ] **Step 5: Verify the repo remains cleanly testable with Redis available**

Run: `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go test ./examples/... -v`
Expected: PASS for all example tests

- [ ] **Step 6: Final commit if verification required doc touch-ups**

```bash
git add README.md docs/lock-scenarios-and-best-practices.md
git commit -m "docs: polish lock scenarios guide verification fixes"
```
