# Composite Fail-If-Held Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `FailIfHeldDef()` so composite members can act as check-only preconditions that abort composite execution when already held, while preserving existing acquire, release, observability, and SDK error-mapping behavior.

**Architecture:** Extend definition-level config in the root SDK, propagate that flag through composite member authoring into runtime `definitions.LockDefinition`, then update composite runtime execution to run in two phases: pre-check all `FailIfHeld` members through `Manager.CheckPresence(...)`, then acquire only normal members. Keep the feature opt-in, reuse existing presence-check infrastructure, and surface a new runtime-to-SDK sentinel mapping for precondition failures.

**Tech Stack:** Go 1.22, root `lockman` SDK, `lockkit` runtime/registry packages, standard `go test` workflow, memory test backend in `lockkit/testkit`

---

## Chunk 1: Definition And Composite Authoring Surface

### Task 1: Add definition-level `FailIfHeld` config in the root SDK

**Files:**
- Modify: `/Users/mrt/workspaces/boilerplate/lockman/definition.go`
- Test: `/Users/mrt/workspaces/boilerplate/lockman/definition_test.go`

- [ ] **Step 1: Write the failing tests for definition config exposure**

Add focused tests in `/Users/mrt/workspaces/boilerplate/lockman/definition_test.go` that prove:
- `FailIfHeldDef()` sets `def.Config().FailIfHeld` to `true`
- `StrictDef()` and `FailIfHeldDef()` can coexist on the same definition
- default definitions return `FailIfHeld == false`

Suggested test names:

```go
func TestFailIfHeldDefSetsDefinitionConfig(t *testing.T) { ... }
func TestFailIfHeldDefCanBeCombinedWithStrictDef(t *testing.T) { ... }
func TestDefinitionConfigDefaultsFailIfHeldToFalse(t *testing.T) { ... }
```

- [ ] **Step 2: Run only the new definition tests to verify they fail**

Run: `go test . -run 'TestFailIfHeldDef|TestDefinitionConfigDefaultsFailIfHeldToFalse' -v`

Expected:
- compile failure because `FailIfHeldDef` or `DefinitionConfig.FailIfHeld` does not exist, or
- assertion failure because the default flag value is wrong

- [ ] **Step 3: Write the minimal implementation in `definition.go`**

In `/Users/mrt/workspaces/boilerplate/lockman/definition.go`:
- add `failIfHeld bool` to `definitionConfig`
- add public `FailIfHeldDef() DefinitionOption`
- update `LockDefinition[T].Config()` to return `DefinitionConfig{Strict: ..., FailIfHeld: ...}`
- add `FailIfHeld bool` to `DefinitionConfig`

- [ ] **Step 4: Run the definition tests again**

Run: `go test . -run 'TestFailIfHeldDef|TestDefinitionConfigDefaultsFailIfHeldToFalse|TestDefineLock' -v`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add /Users/mrt/workspaces/boilerplate/lockman/definition.go /Users/mrt/workspaces/boilerplate/lockman/definition_test.go
git commit -m "feat: add fail-if-held definition option"
```

### Task 2: Add composite authoring validation and local flag propagation

**Files:**
- Modify: `/Users/mrt/workspaces/boilerplate/lockman/binding.go`
- Modify: `/Users/mrt/workspaces/boilerplate/lockman/registry.go`
- Modify: `/Users/mrt/workspaces/boilerplate/lockman/advanced/composite/api.go`
- Test: `/Users/mrt/workspaces/boilerplate/lockman/advanced/composite/api_test.go`
- Test: `/Users/mrt/workspaces/boilerplate/lockman/client_test.go`

- [ ] **Step 1: Write failing tests for authoring-layer validation and propagation**

Add tests that prove:
- `composite.DefineLock(...)` panics on zero members
- `composite.DefineLock(...)` panics on duplicate definitions
- `FailIfHeldDef()` survives composite authoring and is present on the created use case's composite member config before any client/runtime translation

Suggested test names:

```go
func TestDefineLockPanicsOnEmptyComposite(t *testing.T) { ... }
func TestDefineLockPanicsOnDuplicateDefinitions(t *testing.T) { ... }
func TestCompositeAuthoringPropagatesFailIfHeldToUseCaseConfig(t *testing.T) { ... }
```

Place the propagation test in `/Users/mrt/workspaces/boilerplate/lockman/client_test.go`, not `advanced/composite/api_test.go`, so it can legally inspect root-package internals.

For the propagation test, stay entirely at the authoring layer:
- define `parentDef := lockman.DefineLock(..., lockman.FailIfHeldDef())`
- build `transferDef := composite.DefineLock("transfer", parentDef)`
- attach it with `transfer := composite.AttachRun("transfer.run", transferDef)`
- assert `len(transfer.core.config.composite) == 1`
- assert `transfer.core.config.composite[0].failIfHeld == true`

- [ ] **Step 2: Run the composite validation tests to verify they fail**

Run:

```bash
go test ./advanced/composite -run 'TestDefineLockPanicsOnEmptyComposite|TestDefineLockPanicsOnDuplicateDefinitions' -v
go test . -run 'TestCompositeAuthoringPropagatesFailIfHeldToUseCaseConfig' -v
```

Expected:
- failing assertions because empty/duplicate validation does not exist yet
- compile or assertion failure because `failIfHeld` is not yet propagated into use-case composite config

- [ ] **Step 3: Update `binding.go` to carry the authoring flag**

In `/Users/mrt/workspaces/boilerplate/lockman/binding.go`:
- add `failIfHeld bool` to `compositeMemberConfig`
- add `failIfHeld bool` to `CompositeMember[T]`
- update `Member(...)` to inherit both `Strict` and `FailIfHeld` from `def.Config()`
- update `MemberWithStrict(...)` to also copy `def.Config().FailIfHeld`

- [ ] **Step 4: Update `registry.go` to copy the authoring flag into use-case config**

In `/Users/mrt/workspaces/boilerplate/lockman/registry.go`:
- update `newUseCaseCoreWithComposite(...)` so it copies `member.failIfHeld` into `compositeMemberConfig.failIfHeld`

- [ ] **Step 5: Add validation in `advanced/composite/api.go`**

In `/Users/mrt/workspaces/boilerplate/lockman/advanced/composite/api.go`:
- reject zero `defs`
- reject duplicate `LockDefinition` references by stable definition ID
- keep current `MemberWithStrict(...)` pattern so `FailIfHeld` continues to flow through definition config

- [ ] **Step 6: Re-run the authoring tests**

Run:

```bash
go test ./advanced/composite -run 'TestDefineLockPanicsOnEmptyComposite|TestDefineLockPanicsOnDuplicateDefinitions|TestCompositePackageExposesPublicRunUseCaseAuthoring' -v
go test . -run 'TestCompositeAuthoringPropagatesFailIfHeldToUseCaseConfig' -v
```

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add /Users/mrt/workspaces/boilerplate/lockman/binding.go /Users/mrt/workspaces/boilerplate/lockman/registry.go /Users/mrt/workspaces/boilerplate/lockman/advanced/composite/api.go /Users/mrt/workspaces/boilerplate/lockman/advanced/composite/api_test.go /Users/mrt/workspaces/boilerplate/lockman/client_test.go
git commit -m "feat: validate composite definitions"
```

## Chunk 2: Runtime Definition Shape And Error Surface

### Task 3: Extend runtime definition metadata for check-only composite members

**Files:**
- Modify: `/Users/mrt/workspaces/boilerplate/lockman/lockkit/definitions/types.go`
- Modify: `/Users/mrt/workspaces/boilerplate/lockman/client_validation.go`
- Test: `/Users/mrt/workspaces/boilerplate/lockman/client_test.go`

- [ ] **Step 1: Write a failing translation-focused test**

Add a focused root test in `/Users/mrt/workspaces/boilerplate/lockman/client_test.go` that exercises registry/client translation rather than full runtime behavior. Build a composite use case with one `FailIfHeldDef()` member, call into the translation path already covered by the root test helpers, and assert that the resulting runtime member definition has:
- `FailIfHeld == true`
- `CheckOnlyAllowed == true`

If no helper exists, add a small local test helper in `client_test.go` that creates a registry, registers the composite run use case, constructs the client, and inspects the translated lockkit registry definitions.

- [ ] **Step 2: Run the translation-focused test to verify it fails**

Run: `go test . -run 'TestCompositeTranslationSetsFailIfHeldFlags' -v`

Expected:
- compile or assertion failure because translation does not yet set the new fields

- [ ] **Step 3: Update runtime `LockDefinition` shape**

In `/Users/mrt/workspaces/boilerplate/lockman/lockkit/definitions/types.go`:
- add `FailIfHeld bool` to `definitions.LockDefinition`
- keep `CheckOnlyAllowed bool`

- [ ] **Step 4: Update client translation code in `client_validation.go`**

In `/Users/mrt/workspaces/boilerplate/lockman/client_validation.go`:
- in `translateCompositeMemberDefinition(...)`, set `FailIfHeld: member.failIfHeld`
- set `CheckOnlyAllowed: member.failIfHeld`

- [ ] **Step 5: Re-run the translation-focused test**

Run: `go test . -run 'TestCompositeTranslationSetsFailIfHeldFlags' -v`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add /Users/mrt/workspaces/boilerplate/lockman/lockkit/definitions/types.go /Users/mrt/workspaces/boilerplate/lockman/client_validation.go /Users/mrt/workspaces/boilerplate/lockman/client_test.go
git commit -m "feat: translate fail-if-held into runtime definitions"
```

### Task 4: Add runtime and SDK precondition-failed sentinels

**Files:**
- Modify: `/Users/mrt/workspaces/boilerplate/lockman/lockkit/errors/errors.go`
- Modify: `/Users/mrt/workspaces/boilerplate/lockman/errors.go`
- Modify: `/Users/mrt/workspaces/boilerplate/lockman/client_validation.go`
- Test: `/Users/mrt/workspaces/boilerplate/lockman/client_test.go`

- [ ] **Step 1: Write failing tests for engine-error mapping**

Add a focused unit test in `/Users/mrt/workspaces/boilerplate/lockman/client_test.go`:

```go
func TestMapEngineErrorMapsPreconditionFailed(t *testing.T) {
	err := mapEngineError(lockerrors.ErrPreconditionFailed, false)
	if !errors.Is(err, ErrPreconditionFailed) {
		t.Fatalf("expected ErrPreconditionFailed, got %v", err)
	}
}
```

- [ ] **Step 2: Run the mapping test to verify it fails**

Run: `go test . -run 'TestMapEngineErrorMapsPreconditionFailed' -v`

Expected:
- compile failure because one or both sentinels do not exist yet

- [ ] **Step 3: Add runtime sentinel**

In `/Users/mrt/workspaces/boilerplate/lockman/lockkit/errors/errors.go`:
- add `ErrPreconditionFailed = stdErrors.New("precondition failed")`

- [ ] **Step 4: Add SDK sentinel and error mapping**

In `/Users/mrt/workspaces/boilerplate/lockman/errors.go`:
- add `ErrPreconditionFailed = errors.New("lockman: precondition failed")`

In `/Users/mrt/workspaces/boilerplate/lockman/client_validation.go`:
- update `mapEngineError(...)` with:

```go
case errors.Is(err, lockerrors.ErrPreconditionFailed):
	return ErrPreconditionFailed
```

- [ ] **Step 5: Re-run the mapping test**

Run: `go test . -run 'TestMapEngineErrorMapsPreconditionFailed' -v`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add /Users/mrt/workspaces/boilerplate/lockman/lockkit/errors/errors.go /Users/mrt/workspaces/boilerplate/lockman/errors.go /Users/mrt/workspaces/boilerplate/lockman/client_validation.go /Users/mrt/workspaces/boilerplate/lockman/client_test.go
git commit -m "feat: add precondition failed error mapping"
```

## Chunk 3: Composite Runtime Two-Phase Execution

### Task 5: Add pre-check phase for `FailIfHeld` members in composite runtime

**Files:**
- Modify: `/Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite.go`
- Test: `/Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite_test.go`

- [ ] **Step 1: Write a failing runtime test for pre-check before any acquire**

Add a new test in `/Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite_test.go` that proves:
- one member is marked `FailIfHeld` and `CheckOnlyAllowed`
- that member is already held by another manager
- `ExecuteCompositeExclusive(...)` returns `lockerrors.ErrPreconditionFailed`
- callback never runs
- normal member acquires never start

- [ ] **Step 2: Run the new runtime test and verify it fails**

Run: `go test ./lockkit/runtime -run 'TestExecuteCompositeExclusiveFailsPreconditionBeforeAnyAcquire' -v`

Expected:
- returns success or a different error because pre-check logic does not exist yet

- [ ] **Step 3: Add helper registry/request fixtures in `composite_test.go`**

Add small local helpers that mirror existing test style:
- `newFailIfHeldCompositeRegistry(t *testing.T) *registry.Registry`
- `failIfHeldCompositeRequest() definitions.CompositeLockRequest`
- `parentPreconditionRequest() definitions.SyncLockRequest`

- [ ] **Step 4: Implement the two-phase execution in `lockkit/runtime/composite.go`**

Update `/Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite.go` with the smallest practical change:
- keep current plan building and canonicalization
- add a first loop over canonical `plan`
- for each member where `member.Definition.FailIfHeld` is true, call `m.CheckPresence(...)`
- if `status.State == definitions.PresenceHeld`, return an error wrapping `lockerrors.ErrPreconditionFailed`
- include owner info from `status.OwnerID` in the error text
- only after all checks pass, continue into acquire loop for non-`FailIfHeld` members

- [ ] **Step 5: Keep check-only members out of guard installation and acquired lease tracking**

Still in `/Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite.go`:
- only install `guardKeys` for members that will actually acquire
- only call `m.acquireLease(...)` for members where `!member.Definition.FailIfHeld`
- only append those acquired members to `acquired`
- only increment/decrement `activeCounter` for acquired members

- [ ] **Step 6: Re-run the failing runtime test**

Run: `go test ./lockkit/runtime -run 'TestExecuteCompositeExclusiveFailsPreconditionBeforeAnyAcquire' -v`

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add /Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite.go /Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite_test.go
git commit -m "feat: precheck fail-if-held composite members"
```

### Task 6: Verify callback lease semantics and no accounting side effects for check-only members

**Files:**
- Modify: `/Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite_test.go`
- Modify: `/Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite.go`

- [ ] **Step 1: Write failing runtime tests for acquired-only lease payload**

Add tests in `/Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite_test.go` that prove:
- callback `lease.ResourceKeys` includes only acquired members
- `LeaseTTL` and `LeaseDeadline` derive only from acquired members
- precondition members are invisible to the callback lease payload

- [ ] **Step 2: Write failing runtime tests for no guard/accounting side effects**

Add a test that proves:
- `FailIfHeld` members do not trigger reentrancy guard collisions
- `FailIfHeld` members do not increment active-lock metrics
- a composite with only check-only members can still call the callback with an empty acquired lease payload if all preconditions pass

- [ ] **Step 3: Run the targeted runtime tests to verify they fail**

Run: `go test ./lockkit/runtime -run 'TestExecuteCompositeExclusiveExcludesFailIfHeldMembersFromLeaseContext|TestExecuteCompositeExclusiveDoesNotTrackFailIfHeldMembersAsActive|TestExecuteCompositeExclusiveAllowsAllPreconditionsComposite' -v`

Expected:
- one or more assertions fail because lease payload or guard/accounting logic still includes check-only members

- [ ] **Step 4: Adjust `buildCompositeLeaseContext(...)` and surrounding execution only if needed**

In `/Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite.go`:
- prefer leaving `buildCompositeLeaseContext(...)` unchanged if `acquired` already contains only real acquires
- if tests reveal empty-acquired behavior needs an explicit branch, make the smallest possible adjustment

- [ ] **Step 5: Re-run the targeted runtime tests**

Run: `go test ./lockkit/runtime -run 'TestExecuteCompositeExclusiveExcludesFailIfHeldMembersFromLeaseContext|TestExecuteCompositeExclusiveDoesNotTrackFailIfHeldMembersAsActive|TestExecuteCompositeExclusiveAllowsAllPreconditionsComposite' -v`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add /Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite.go /Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite_test.go
git commit -m "test: cover fail-if-held composite lease semantics"
```

## Chunk 4: Public SDK Behavior And Regression Coverage

### Task 7: Finish public composite API behavior tests

**Files:**
- Modify: `/Users/mrt/workspaces/boilerplate/lockman/advanced/composite/api_test.go`
- Modify: `/Users/mrt/workspaces/boilerplate/lockman/client_test.go`

- [ ] **Step 1: Add the full public API coverage in `advanced/composite/api_test.go`**

Add or complete these tests in `/Users/mrt/workspaces/boilerplate/lockman/advanced/composite/api_test.go`:
- `TestCompositePackageFailIfHeldCheckPassesWhenNotHeld`
- `TestCompositePackageFailIfHeldCheckAbortsWhenHeld`
- `TestCompositePackageFailIfHeldErrorIncludesOwnerInfo`
- `TestCompositePackageFailIfHeldMembersAreExcludedFromLeasePayload`
- `TestDefineLockPanicsOnEmptyComposite`
- `TestDefineLockPanicsOnDuplicateDefinitions`

- [ ] **Step 2: Run the public composite test file**

Run: `go test ./advanced/composite -v`

Expected: PASS

- [ ] **Step 3: Keep the root SDK regression for `mapEngineError` green**

Run: `go test . -run 'TestMapEngineError|TestCompositeTranslationSetsFailIfHeldFlags' -v`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add /Users/mrt/workspaces/boilerplate/lockman/advanced/composite/api_test.go /Users/mrt/workspaces/boilerplate/lockman/client_test.go
git commit -m "test: cover public fail-if-held composite behavior"
```

### Task 8: Run broader verification for touched packages and repo-level safety

**Files:**
- Verify only:
  - `/Users/mrt/workspaces/boilerplate/lockman/definition.go`
  - `/Users/mrt/workspaces/boilerplate/lockman/binding.go`
  - `/Users/mrt/workspaces/boilerplate/lockman/advanced/composite/api.go`
  - `/Users/mrt/workspaces/boilerplate/lockman/errors.go`
  - `/Users/mrt/workspaces/boilerplate/lockman/client_validation.go`
  - `/Users/mrt/workspaces/boilerplate/lockman/lockkit/definitions/types.go`
  - `/Users/mrt/workspaces/boilerplate/lockman/lockkit/errors/errors.go`
  - `/Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite.go`
  - `/Users/mrt/workspaces/boilerplate/lockman/definition_test.go`
  - `/Users/mrt/workspaces/boilerplate/lockman/advanced/composite/api_test.go`
  - `/Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite_test.go`
  - `/Users/mrt/workspaces/boilerplate/lockman/client_test.go`

- [ ] **Step 1: Run the fast targeted package tests**

Run:

```bash
go test . -run 'TestFailIfHeldDef|TestDefinitionConfigDefaultsFailIfHeldToFalse|TestMapEngineError|TestCompositeTranslationSetsFailIfHeldFlags' -v
go test ./advanced/composite -v
go test ./lockkit/runtime -run 'TestExecuteCompositeExclusive' -v
```

Expected: PASS

- [ ] **Step 2: Run broader compile and package checks that match touched areas**

Run:

```bash
go test ./... -run '^$'
go test -tags lockman_examples ./examples/... -run '^$'
```

Expected: PASS

- [ ] **Step 3: Run the repository’s CI-parity commands from `AGENTS.md`**

Run:

```bash
go test ./...
GOWORK=off go test ./...
go test ./backend/redis/...
go test ./idempotency/redis/...
go test ./guard/postgres/...
go test -tags lockman_examples ./examples/... -run '^$'
```

Expected: PASS

- [ ] **Step 4: Run formatting and hygiene checks**

Run:

```bash
gofmt -w /Users/mrt/workspaces/boilerplate/lockman/definition.go /Users/mrt/workspaces/boilerplate/lockman/binding.go /Users/mrt/workspaces/boilerplate/lockman/advanced/composite/api.go /Users/mrt/workspaces/boilerplate/lockman/errors.go /Users/mrt/workspaces/boilerplate/lockman/client_validation.go /Users/mrt/workspaces/boilerplate/lockman/lockkit/definitions/types.go /Users/mrt/workspaces/boilerplate/lockman/lockkit/errors/errors.go /Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite.go /Users/mrt/workspaces/boilerplate/lockman/definition_test.go /Users/mrt/workspaces/boilerplate/lockman/advanced/composite/api_test.go /Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite_test.go /Users/mrt/workspaces/boilerplate/lockman/client_test.go
go test ./... -run '^$'
```

Expected: PASS and no formatting diffs remain

- [ ] **Step 5: Final commit**

```bash
git add /Users/mrt/workspaces/boilerplate/lockman/definition.go /Users/mrt/workspaces/boilerplate/lockman/binding.go /Users/mrt/workspaces/boilerplate/lockman/advanced/composite/api.go /Users/mrt/workspaces/boilerplate/lockman/errors.go /Users/mrt/workspaces/boilerplate/lockman/client_validation.go /Users/mrt/workspaces/boilerplate/lockman/lockkit/definitions/types.go /Users/mrt/workspaces/boilerplate/lockman/lockkit/errors/errors.go /Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite.go /Users/mrt/workspaces/boilerplate/lockman/definition_test.go /Users/mrt/workspaces/boilerplate/lockman/advanced/composite/api_test.go /Users/mrt/workspaces/boilerplate/lockman/lockkit/runtime/composite_test.go /Users/mrt/workspaces/boilerplate/lockman/client_test.go
git commit -m "feat: add composite fail-if-held preconditions"
```

---

## Implementation Notes For The Executor

- Keep the change minimal. Do not redesign composite execution around a new abstraction unless the current function becomes impossible to reason about.
- Reuse `Manager.CheckPresence(...)` exactly as the spec requires. Do not bypass it and call the backend directly.
- Preserve canonical member ordering by continuing to use `policy.CanonicalizeMembers(...)` before pre-check and acquire phases.
- The tricky part is preserving enough per-member data to call `CheckPresence(...)` after canonicalization. Solve that with a small composite-local struct rather than broad API changes.
- Keep owner information in the returned error text for held preconditions, but keep `errors.Is(err, lockerrors.ErrPreconditionFailed)` and `errors.Is(err, lockman.ErrPreconditionFailed)` working.
- Do not let `FailIfHeld` members affect:
  - `m.active`
  - active-lock counters
  - `acquired` slice contents
  - reverse-order release
  - callback lease payload
- `FailIfHeld + StrictDef` is allowed at definition authoring time, but existing composite strict-member validation still applies. Do not weaken the current strict-member rejection behavior unless a failing test proves the spec requires it.
