# Shorthand Constructor Deprecation Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deprecate the root-SDK shorthand constructors while keeping behavior unchanged and aligning docs/examples around the definition-first model.

**Architecture:** The change stays intentionally shallow. Add deprecation comments to the three shorthand constructors, then update the bounded documentation surface so new code is always taught through `DefineLock + ...On`, while shorthand is described as deprecated but still fully functional in the current release line. Do not redesign advanced wrappers or change runtime behavior.

**Tech Stack:** Go 1.22, Markdown docs, root SDK examples, nested Go modules, `go test`

---

## File Map

| File | Responsibility |
|---|---|
| `usecase_run.go` | Add Go deprecation comment to `DefineRun` |
| `usecase_hold.go` | Add Go deprecation comment to `DefineHold` |
| `usecase_claim.go` | Add Go deprecation comment to `DefineClaim` |
| `README.md` | Make shorthand explicitly deprecated in the root SDK narrative |
| `CHANGELOG.md` | Record shorthand deprecation and next-major removal intent |
| `docs/quickstart-sync.md` | Keep sync quickstart definition-first and mark shorthand deprecated |
| `docs/quickstart-async.md` | Keep async quickstart definition-first and mark shorthand deprecated |
| `docs/production-guide.md` | Reinforce root-SDK policy and note advanced packages are out of scope |
| `docs/runtime-vs-workers.md` | Keep execution-surface framing aligned with deprecated shorthand policy |
| `docs/lock-definition-reference.md` | Carry the clearest migration examples from shorthand to `...On` |
| `docs/registry-and-usecases.md` | Reinforce definition-first registration and shorthand deprecation wording |
| `examples/README.md` | Keep `shared-lock-definition` as the starting point and reframe shorthand examples |
| `examples/sdk/shared-lock-definition/README.md` | Stay the canonical non-deprecated starting point |
| `examples/sdk/sync-approve-order/README.md` | Reframe shorthand sync example as deprecated compatibility coverage |
| `examples/sdk/async-process-order/README.md` | Reframe shorthand async example as deprecated compatibility coverage |
| `examples/sdk/shared-aggregate-split-definitions/README.md` | Ensure shorthand wording is not presented as recommended new code |
| `examples/sdk/parent-lock-over-composite/README.md` | Ensure shorthand wording is not presented as recommended new code |
| `examples/sdk/sync-fenced-write/README.md` | Ensure shorthand wording is not presented as recommended new code |

## Chunk 1: Code-Level Deprecation Markers

### Task 1: Add Go deprecation comments to shorthand constructors

**Files:**
- Modify: `usecase_run.go:14-18`
- Modify: `usecase_hold.go:24-28`
- Modify: `usecase_claim.go:14-18`
- Spec: `docs/superpowers/specs/2026-04-03-shorthand-constructor-deprecation-design.md`

- [ ] **Step 1: Add deprecation comment to `DefineRun`**

```go
// Deprecated: use DefineLock plus DefineRunOn.
func DefineRun[T any](name string, binding Binding[T], opts ...UseCaseOption) RunUseCase[T] {
```

- [ ] **Step 2: Add deprecation comment to `DefineHold`**

```go
// Deprecated: use DefineLock plus DefineHoldOn.
func DefineHold[T any](name string, binding Binding[T], opts ...UseCaseOption) HoldUseCase[T] {
```

- [ ] **Step 3: Add deprecation comment to `DefineClaim`**

```go
// Deprecated: use DefineLock plus DefineClaimOn.
func DefineClaim[T any](name string, binding Binding[T], opts ...UseCaseOption) ClaimUseCase[T] {
```

- [ ] **Step 4: Verify the comments are exact**

Read back the three files and confirm the deprecation text matches the spec exactly.

- [ ] **Step 5: Run focused package tests**

Run: `go test ./... -run '^$'`
Expected: PASS compile-only check for all root packages with no behavior changes.

## Chunk 2: Root Docs And Migration Story

### Task 2: Harden root README deprecation stance

**Files:**
- Modify: `README.md`
- Spec: `docs/superpowers/specs/2026-04-03-shorthand-constructor-deprecation-design.md`

- [ ] **Step 1: Rewrite the shorthand section in `README.md`**

Required wording outcome:
- shorthand is deprecated
- shorthand remains fully functional in the current release line
- new code should use `DefineLock + ...On`
- the next major release removes shorthand from the root SDK

Target section: the current shorthand guidance section below the definition-first happy path.

- [ ] **Step 2: Verify the first substantial code sample remains definition-first**

Check that the first multi-line SDK code block in `README.md` still includes `DefineLock` and at least one of `DefineRunOn`, `DefineHoldOn`, or `DefineClaimOn`.

- [ ] **Step 3: Verify `README.md` does not present shorthand as recommended new code**

Read the file top to bottom and confirm no shorthand snippet or prose is positioned as the preferred starter path.

### Task 3: Add explicit migration guidance to the reference docs

**Files:**
- Modify: `docs/lock-definition-reference.md`
- Modify: `docs/registry-and-usecases.md`
- Modify: `docs/runtime-vs-workers.md`
- Spec: `docs/superpowers/specs/2026-04-03-shorthand-constructor-deprecation-design.md`

- [ ] **Step 1: Add mechanical migration examples to `docs/lock-definition-reference.md`**

Required outcome:
- `DefineRun(...)` -> `DefineLock(...)` + `DefineRunOn(...)`
- `DefineHold(...)` -> `DefineLock(...)` + `DefineHoldOn(...)`
- `DefineClaim(...)` -> `DefineLock(...)` + `DefineClaimOn(...)`

Target section: the current reference sections that describe sync and async use cases.

- [ ] **Step 2: Add the three required migration clarifications**

Required content:
- the extracted `DefineLock(...)` may remain private to one package
- sharing is not required to justify migration
- the value of migration is one consistent API model, not only shared identity reuse

- [ ] **Step 3: Update `docs/registry-and-usecases.md`**

Required outcome:
- definition-first registration remains the only recommended model
- shorthand, if mentioned, is described as deprecated but still functional for compatibility

Target section: `Define In Code` and any follow-up prose that currently treats shorthand as normal authoring.

- [ ] **Step 4: Update `docs/runtime-vs-workers.md`**

Required outcome:
- `Run` and `Claim` remain framed as execution surfaces over the definition-first model
- no wording suggests shorthand is the normal place to start

Target section: intro paragraph and examples section.

- [ ] **Step 5: Read through the three files for wording consistency**

Confirm they all mean:
- shorthand is deprecated
- shorthand still works in the current release line
- new code should use `DefineLock + ...On`

## Chunk 3: Quickstarts, Production Guide, And Examples Positioning

### Task 4: Reframe quickstarts and production guide

**Files:**
- Modify: `docs/quickstart-sync.md`
- Modify: `docs/quickstart-async.md`
- Modify: `docs/production-guide.md`
- Spec: `docs/superpowers/specs/2026-04-03-shorthand-constructor-deprecation-design.md`

- [ ] **Step 1: Update `docs/quickstart-sync.md`**

Required outcome:
- first code block remains definition-first
- any shorthand mention is labeled deprecated
- shorthand mention explicitly directs readers to `DefineLock + ...On` for new code

Target sections: opening narrative, first code block, and runnable-examples guidance.

- [ ] **Step 2: Update `docs/quickstart-async.md`**

Required outcome:
- first code block remains definition-first
- any shorthand mention is labeled deprecated
- shorthand mention explicitly directs readers to `DefineLock + ...On` for new code

Target sections: opening narrative, first code block, and runnable-examples guidance.

- [ ] **Step 3: Update `docs/production-guide.md`**

Required outcome:
- root SDK path recommends only `DefineLock + ...On` for new code
- advanced wrappers are explicitly stated to be outside the scope of this root-SDK shorthand deprecation pass

Target sections: `Start Here`, quickstart links area, and root-SDK guidance section.

- [ ] **Step 4: Read through the three files for consistent deprecation wording**

### Task 5: Reframe example index and shorthand example READMEs

**Files:**
- Modify: `examples/README.md`
- Modify: `examples/sdk/shared-lock-definition/README.md`
- Modify: `examples/sdk/sync-approve-order/README.md`
- Modify: `examples/sdk/async-process-order/README.md`
- Modify: `examples/sdk/shared-aggregate-split-definitions/README.md`
- Modify: `examples/sdk/parent-lock-over-composite/README.md`
- Modify: `examples/sdk/sync-fenced-write/README.md`
- Spec: `docs/superpowers/specs/2026-04-03-shorthand-constructor-deprecation-design.md`

- [ ] **Step 1: Update `examples/README.md`**

Required outcome:
- `shared-lock-definition` remains the starting point
- shorthand examples are not described as recommended starter code

Target sections: `Start Here`, execution-surface guidance, and any summary text that describes learning order.

- [ ] **Step 2: Update `examples/sdk/sync-approve-order/README.md`**

Required outcome:
- shorthand is explicitly labeled deprecated
- shorthand is explicitly described as retained for compatibility in the current release line

Target sections: `Backbone concept`, `What this example defines`, and `Why this shape matters`.

- [ ] **Step 3: Update `examples/sdk/async-process-order/README.md`**

Required outcome:
- shorthand is explicitly labeled deprecated
- shorthand is explicitly described as retained for compatibility in the current release line

Target sections: `Backbone concept`, `What this example defines`, and `Why this shape matters`.

- [ ] **Step 4: Update `examples/sdk/shared-lock-definition/README.md` if needed**

Required outcome:
- it remains the canonical non-deprecated starting point
- no text suggests shorthand is an equally preferred starter path

- [ ] **Step 5: Update `examples/sdk/shared-aggregate-split-definitions/README.md`**

Required outcome:
- if shorthand remains in the example source, the README frames it as deprecated compatibility or legacy coverage rather than preferred new code

- [ ] **Step 6: Update `examples/sdk/parent-lock-over-composite/README.md`**

Required outcome:
- if shorthand remains in the example source, the README frames it as deprecated compatibility or legacy coverage rather than preferred new code

- [ ] **Step 7: Update `examples/sdk/sync-fenced-write/README.md`**

Required outcome:
- if shorthand remains in the example source, the README frames it as deprecated compatibility or legacy coverage rather than preferred new code

## Chunk 4: Changelog And Final Verification

### Task 6: Record release intent and run full verification

**Files:**
- Modify: `CHANGELOG.md`
- Spec: `docs/superpowers/specs/2026-04-03-shorthand-constructor-deprecation-design.md`

- [ ] **Step 1: Add a changelog entry for shorthand deprecation**

Required content:
- shorthand constructors are deprecated now
- `DefineLock + ...On` is the path for new code
- shorthand behavior is unchanged in the current line
- shorthand removal is planned for the next major release

- [ ] **Step 2: Run workspace test suite**

Run: `go test ./...`
Expected: PASS

- [ ] **Step 3: Run non-workspace root suite**

Run: `GOWORK=off go test ./...`
Expected: PASS

- [ ] **Step 4: Run nested module suites**

Run: `go test ./backend/redis/...`
Expected: PASS

Run: `go test ./idempotency/redis/...`
Expected: PASS

Run: `go test ./guard/postgres/...`
Expected: PASS

- [ ] **Step 5: Compile tagged examples**

Run: `go test -tags lockman_examples ./examples/... -run '^$'`
Expected: PASS

- [ ] **Step 6: Perform final read-through checks**

Verify manually:
- the three shorthand functions carry the exact deprecation comments
- the first substantial README code sample is still definition-first
- required docs say shorthand is deprecated but still functional in the current release line
- no required doc presents shorthand as recommended new code
- `README.md` or `docs/lock-definition-reference.md` contains all three mechanical migration mappings for `DefineRun`, `DefineHold`, and `DefineClaim`
- `examples/sdk/sync-approve-order/README.md` explicitly labels shorthand deprecated and compatibility-only
- `examples/sdk/async-process-order/README.md` explicitly labels shorthand deprecated and compatibility-only
- either `README.md` or `docs/production-guide.md` states that advanced packages are outside the scope of this root-SDK shorthand deprecation pass
- no files outside the bounded acceptance surface were modified unless the user explicitly expanded scope
- changelog promises future removal without implying current breakage
