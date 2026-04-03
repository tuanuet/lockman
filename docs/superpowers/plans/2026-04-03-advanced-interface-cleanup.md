# Advanced Interface Cleanup Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current `advanced/strict` and `advanced/composite` wrapper-style APIs with a definition-first advanced surface that cleanly matches the root SDK model.

**Architecture:** Keep `DefineRunOn` semantically stable across the public API by making `advanced/strict` attach strict behavior onto an existing `LockDefinition`, and by reshaping composite authoring around real child `LockDefinition` values instead of `DefineMember` wrappers. Remove the old advanced exported constructors in the same change that updates tests and docs so the repository never teaches both models at once.

**Tech Stack:** Go 1.22, root `lockman` module, Go tests, Markdown docs

---

## File Map

### Core API Files

- Modify: `advanced/strict/api.go`
  Responsibility: replace `strict.DefineRun(...)` with `strict.DefineRunOn(...)`.
- Modify: `advanced/strict/api_test.go`
  Responsibility: verify strict advanced authoring uses `LockDefinition` plus `DefineRunOn` and still yields fencing tokens.
- Modify: `advanced/composite/api.go`
  Responsibility: replace `DefineMember` and `DefineRun*` wrappers with the new composite surface, or reduce the package to the chosen minimal advanced definition builder if root primitives absorb more of the API.
- Modify: `advanced/composite/api_test.go`
  Responsibility: verify the new composite authoring shape, ordered resource acquisition, and error semantics.
- Modify: `advanced/composite/doc.go`
  Responsibility: package doc alignment with definition-first composite authoring.
- Modify: `advanced/strict/doc.go`
  Responsibility: package doc alignment with definition-first strict authoring.

### Root Support Files

- Inspect and possibly modify: `definition.go`
  Responsibility: determine whether composite definitions can be represented as real `lockman.LockDefinition[T]` values.
- Inspect and possibly modify: `binding.go`
  Responsibility: reuse or minimally extend existing root composite helpers internally if needed, without changing their public root-SDK status in this pass.
- Inspect and possibly modify: `usecase_run.go`
  Responsibility: ensure `DefineRunOn` and run binding behavior support the new advanced shape without leaking shorthand assumptions.
- Inspect and possibly modify: `client_validation.go`
  Responsibility: keep validation coherent if composite definitions are represented differently after the cleanup.
- Inspect and possibly modify: tests such as `client_test.go`, `definition_test.go`
  Responsibility: update expectations if exported API names or composite representation change.

- Do not remove or redesign root exports such as `lockman.Member`, `lockman.DefineCompositeRun`, `lockman.CompositeMember`, or `lockman.Composite` in this plan unless a minimal internal change is strictly required to make the advanced surface compile and pass tests.

### Documentation Files

- Modify: `docs/advanced/strict.md`
  Responsibility: document `strict.DefineRunOn(name, def, ...)`.
- Modify: `docs/advanced/composite.md`
  Responsibility: document composite child definitions as `lockman.DefineLock(...)` values.
- Modify: `README.md`
  Responsibility: only if advanced links or examples would otherwise contradict the new advanced APIs.
- Modify: example READMEs or source files that demonstrate removed advanced names, only if they are directly affected by the cleanup.

### Plan Constraints

- Keep the preferred target from the spec: `composite.DefineLock(...)` should return a real `lockman.LockDefinition[T]` if technically viable.
- If that is not technically viable, implement the fallback from the spec without reusing the name `DefineRunOn` inside `advanced/composite`.
- Remove old advanced exported constructors atomically with docs and tests.

## Chunk 1: Strict Surface Replacement

### Task 1: Lock down the desired strict API with tests

**Files:**
- Modify: `advanced/strict/api_test.go`
- Inspect: `advanced/strict/api.go`

- [ ] **Step 1: Replace the old test shape with the new `DefineRunOn` authoring path**

Update the test to construct a definition first:

```go
approveDef := lockman.DefineLock(
	"order.strict-write",
	lockman.BindResourceID("order", func(v string) string { return v }),
)
approve := DefineRunOn("order.strict-write", approveDef)
```

- [ ] **Step 2: Add an explicit assertion that strict behavior survives duplicate strict options**

Extend the test so the constructor is called with `lockman.Strict()` in options too:

```go
approve := DefineRunOn("order.strict-write", approveDef, lockman.Strict())
```

Then verify the run still succeeds and returns fencing tokens that increase across owners.

- [ ] **Step 3: Run the strict package test and verify the old API no longer compiles once removed**

Run: `go test ./advanced/strict -run '^TestStrictPackageExposesPublicRunUseCaseAuthoring$' -v`

Expected: PASS after the implementation step, and no remaining references to `strict.DefineRun` in package tests.

### Task 2: Implement `strict.DefineRunOn` and remove `strict.DefineRun`

**Files:**
- Modify: `advanced/strict/api.go`
- Modify: `advanced/strict/doc.go`
- Test: `advanced/strict/api_test.go`

- [ ] **Step 1: Replace the exported constructor in `advanced/strict/api.go`**

Target implementation:

```go
func DefineRunOn[T any](name string, def lockman.LockDefinition[T], opts ...lockman.UseCaseOption) lockman.RunUseCase[T] {
	return lockman.DefineRunOn(name, def, append(opts, lockman.Strict())...)
}
```

- [ ] **Step 2: Remove the old `DefineRun` export from the package**

There should be no remaining exported `strict.DefineRun` symbol in the target state.

- [ ] **Step 3: Update package docs to teach the new shape**

Package doc text should describe `advanced/strict` as attaching strict run semantics onto an existing definition-first flow.

- [ ] **Step 4: Run the strict package tests**

Run: `go test ./advanced/strict/...`

Expected: PASS.

## Chunk 2: Composite Surface Redesign

### Task 3: Decide whether composite definitions can be true `lockman.LockDefinition[T]` values

**Files:**
- Inspect: `definition.go`
- Inspect: `binding.go`
- Inspect: `usecase_run.go`
- Inspect: `client_validation.go`
- Inspect: `client_test.go`

- [ ] **Step 1: Trace how `LockDefinition[T]` identity and binding are consumed today**

Confirm whether a composite definition can carry enough metadata through `definitionRef`, binding, and use-case normalization without special-casing that would distort the root definition abstraction.

- [ ] **Step 2: Choose the implementation branch before editing public composite APIs**

Decision rule:

1. use the preferred path if `composite.DefineLock(...)` can return a true `lockman.LockDefinition[T]` with minimal internal changes and no root public API redesign
2. use the fallback path only if the preferred path would require distorting `LockDefinition[T]` semantics or adding a second hidden public model behind the same name

- [ ] **Step 3: Run a targeted compile check before editing broad surfaces**

Run: `go test ./... -run '^$'`

Expected: PASS before composite changes, giving a clean compile baseline.

### Task 4: Write failing composite tests for the preferred design first

**Files:**
- Modify: `advanced/composite/api_test.go`
- Inspect: `binding.go`

- [ ] **Step 1: Rewrite the main composite test to define child locks with `lockman.DefineLock(...)`**

Use a shared input type like:

```go
type transferInput struct {
	AccountID string
	LedgerID  string
}
```

and child definitions like:

```go
accountDef := lockman.DefineLock("account", lockman.BindResourceID("account", func(in transferInput) string { return in.AccountID }))
ledgerDef := lockman.DefineLock("ledger", lockman.BindResourceID("ledger", func(in transferInput) string { return in.LedgerID }))
```

- [ ] **Step 2: Author the new preferred composite shape in the test**

Preferred expectation:

```go
transferDef := DefineLock("transfer", accountDef, ledgerDef)
transfer := lockman.DefineRunOn("transfer.run", transferDef, lockman.TTL(5*time.Second))
```

Keep the existing runtime assertions for ordered `lease.ResourceKeys`.

- [ ] **Step 3: Add a failing compile-path test or package-level assertion for removed exports**

Add a breaking-change verification file outside the removed packages, for example under `internal/compilecheck/` or another test-only location, that attempts to reference the removed advanced exports behind a build tag or scripted compile check. The verification command should prove those names no longer exist after the change.

- [ ] **Step 4: Run the composite package test to capture the initial failure**

Run: `go test ./advanced/composite -run '^TestCompositePackageExposesPublicRunUseCaseAuthoring$' -v`

Expected: FAIL until the new API is implemented.

### Task 5: Implement the preferred composite API if technically viable

**Files:**
- Modify: `advanced/composite/api.go`
- Modify only the minimal root files needed from this set: `definition.go`, `binding.go`, `usecase_run.go`, `client_validation.go`
- Test: `advanced/composite/api_test.go`

- [ ] **Step 1: Introduce `composite.DefineLock(name, defs...)` around root child definitions**

Preferred target:

```go
func DefineLock[T any](name string, defs ...lockman.LockDefinition[T]) lockman.LockDefinition[T]
```

Implementation must preserve:

1. child definition order
2. each child definition's stable identity
3. one shared input type `T`
4. compatibility with root `lockman.DefineRunOn(...)`

- [ ] **Step 2: Remove `DefineMember`, `DefineRun`, and `DefineRunWithOptions` exports from `advanced/composite`**

There should be no remaining exported wrapper constructors using those names.

- [ ] **Step 3: Make the minimal root changes needed to support the preferred shape**

Possible implementation directions to evaluate:

1. store composite-definition metadata in `definitionRef` and reuse `DefineRunOn`
2. bridge `composite.DefineLock` into existing composite member machinery inside root normalization
3. reuse existing root composite machinery internally where possible, without changing its root public exports in this pass

Do not add a second public authoring model if the preferred shape can be made to work with a small internal extension.

- [ ] **Step 3a: Keep the root public surface stable while changing internals**

Explicit rule:

1. do not rename, remove, or redesign root public composite exports in this task
2. limit root edits to what is necessary for advanced composite support and test correctness
3. if the preferred path requires broader root-API redesign, stop and switch to the fallback branch instead

- [ ] **Step 4: Run the composite package tests**

Run: `go test ./advanced/composite/...`

Expected: PASS.

### Task 6: Implement the fallback only if the preferred composite shape proves impossible

**Files:**
- Modify: `advanced/composite/api.go`
- Modify: `advanced/composite/api_test.go`
- Modify: docs that teach composite

- [ ] **Step 1: Introduce a dedicated advanced composite definition type**

Fallback shape:

```go
type Definition[T any] struct { ... }

func DefineLock[T any](name string, defs ...lockman.LockDefinition[T]) Definition[T]
func AttachRun[T any](name string, def Definition[T], opts ...lockman.UseCaseOption) lockman.RunUseCase[T]
```

- [ ] **Step 2: Keep the fallback naming constraints from the spec**

Rules:

1. do not reintroduce `DefineMember`
2. do not export `DefineRun` or `DefineRunWithOptions`
3. do not reuse the name `DefineRunOn` in the fallback composite package

- [ ] **Step 3: Update the composite tests and docs to the fallback shape only if the preferred path is impossible**

If fallback is used, the docs must clearly explain why composite has its own definition abstraction.

- [ ] **Step 4: Run the composite package tests again**

Run: `go test ./advanced/composite/...`

Expected: PASS.

## Chunk 3: Documentation And Repository Consistency

### Task 7: Rewrite advanced docs to teach only the new API

**Files:**
- Modify: `docs/advanced/strict.md`
- Modify: `docs/advanced/composite.md`
- Modify: `advanced/strict/doc.go`
- Modify: `advanced/composite/doc.go`

- [ ] **Step 1: Rewrite the strict docs around `DefineLock + strict.DefineRunOn`**

Required example shape:

```go
approveDef := lockman.DefineLock("order", ...)
approve := strict.DefineRunOn("order.strict-write", approveDef)
```

- [ ] **Step 2: Rewrite the composite docs around child `lockman.DefineLock(...)` values**

Preferred example shape:

```go
accountDef := lockman.DefineLock("account", ...)
ledgerDef := lockman.DefineLock("ledger", ...)
transferDef := composite.DefineLock("transfer", accountDef, ledgerDef)
transfer := lockman.DefineRunOn("transfer.run", transferDef)
```

Fallback example shape only if required:

```go
transferDef := composite.DefineLock("transfer", accountDef, ledgerDef)
transfer := composite.AttachRun("transfer.run", transferDef)
```

The composite docs must also say explicitly that child definitions may stay private inside one package and that reuse is optional, not mandatory.

- [ ] **Step 3: Update package docs to remove shorthand-oriented wording**

The package docs should describe advanced packages as definition-first extensions, not convenience wrappers.

- [ ] **Step 4: Run a docs grep for removed advanced constructor names**

Run: `rg 'strict\.DefineRun|composite\.DefineMember|composite\.DefineRunWithOptions|composite\.DefineRun' .`

Expected: no remaining user-facing docs or examples that teach removed names, except possibly historical/spec files that intentionally discuss the migration.

### Task 7b: Prove removed advanced exports are gone

**Files:**
- Create or modify: a test-only compile-check location such as `internal/compilecheck/` or a temporary verification script under repository conventions

- [ ] **Step 1: Add a compile check that references the removed names**

Include references to:

```go
strict.DefineRun
composite.DefineMember
composite.DefineRun
composite.DefineRunWithOptions
```

The check should be structured so that it fails to compile once the removal is correct.

- [ ] **Step 2: Run the compile check as part of verification**

Use a command that proves those names are unresolved after the cleanup, then remove any temporary verification artifact if repository conventions require it.

### Task 8: Update any affected example code or repository references

**Files:**
- Inspect: `examples/sdk/sync-fenced-write/**`
- Inspect: `examples/sdk/sync-transfer-funds/**`
- Inspect: `backend/redis/examples/sync-fenced-write/**`
- Inspect: `backend/redis/examples/sync-transfer-funds/**`
- Inspect: any root docs that link to advanced APIs

- [ ] **Step 1: Search for all usages of removed advanced exported names**

Run: `rg 'strict\.DefineRun|composite\.DefineMember|composite\.DefineRunWithOptions|composite\.DefineRun' .`

- [ ] **Step 2: Update only the directly affected example files and docs**

Keep the changes minimal and aligned with the chosen preferred or fallback composite design.

Do not expand this task into a general root composite API rewrite.

- [ ] **Step 3: Run targeted example compile checks for touched examples**

Examples:

Run: `go test -tags lockman_examples ./examples/... -run '^$'`

If backend adapter examples changed too, also run the smallest matching compile/test commands for those paths.

## Chunk 4: Verification

### Task 9: Run targeted and repository-level verification

**Files:**
- No file edits; verification only

- [ ] **Step 1: Run the directly affected package tests**

Run:

```bash
go test ./advanced/strict/...
go test ./advanced/composite/...
```

Expected: PASS.

- [ ] **Step 2: Run root compile and test coverage relevant to the API change**

Run:

```bash
go test ./... -run '^$'
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run CI-parity commands from `AGENTS.md` if composite support touched root behavior**

Run:

```bash
GOWORK=off go test ./...
go test ./backend/redis/...
go test ./idempotency/redis/...
go test ./guard/postgres/...
go test -tags lockman_examples ./examples/... -run '^$'
```

Expected: PASS.

- [ ] **Step 4: Review the final diff for accidental surface expansion**

Check that the final API only includes the intended new advanced names and does not leave stray exported compatibility wrappers behind.
