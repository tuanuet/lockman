# Shared Lock Definition Design Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add definition-first shared lock identities so multiple use cases can share one lock definition across run, claim, hold, and composite flows while preserving shorthand APIs.

**Architecture:** Introduce a new `LockDefinition[T]` SDK primitive that owns stable identity, binding, and definition-level strictness. Public use cases become execution surfaces that reference a definition directly (`DefineRunOn`, `DefineHoldOn`, `DefineClaimOn`) or implicitly create a private definition through existing shorthand constructors. Client planning must register unique engine definitions separately from public use cases, normalize shared definitions to `ExecutionBoth` when needed, and adapt composite members to reference existing definitions through member-specific projections.

**Tech Stack:** Go 1.22, existing `lockkit/definitions`, `lockkit/registry`, `internal/sdk`, `backend.Driver`, Redis backend optional capabilities.

---

## File Map

### Create

| File | Responsibility |
|------|----------------|
| `definition.go` | `LockDefinition[T]`, `DefineLock`, definition ID helpers, `ForceRelease` |
| `definition_test.go` | Unit tests for shared definitions, explicit constructors, strict-definition validation, force release |
| `examples/core/shared-lock-definition/main.go` | Example of multiple use cases sharing one definition |
| `examples/core/shared-lock-definition/main_test.go` | Compile-level/example behavior coverage |

### Modify

| File | Responsibility |
|------|----------------|
| `binding.go` | Split definition options from use-case options; replace composite member API with definition+projection builder |
| `registry.go` | Store shared definition metadata on `useCaseCore`; support collecting unique definitions |
| `usecase_run.go` | Add `DefineRunOn`; keep shorthand `DefineRun`; update public `DefinitionID()` behavior if needed to remain name-facing |
| `usecase_hold.go` | Add `DefineHoldOn`; keep shorthand `DefineHold`; reject strict definitions for hold at constructor time and preserve startup validation |
| `usecase_claim.go` | Add `DefineClaimOn`; keep shorthand `DefineClaim` |
| `client_validation.go` | Preserve existing lineage behavior while registering unique engine definitions; compute execution kind across shared definitions; adapt composite translation |
| `internal/sdk/usecase.go` | Accept explicit definition IDs when normalizing public use cases |
| `client_run.go` | Ensure runtime execution uses shared definition identity for standalone and composite run requests |
| `client_hold.go` | Ensure hold execution uses shared definition identity |
| `client_claim.go` | Ensure claim execution uses shared definition identity |
| `backend/contracts.go` | Add optional force-release capability |
| `backend/redis/driver.go` | Implement force release by definition ID and resource key, including strict-state cleanup |
| `client_test.go` | Add client-plan validation cases for shared definitions, `ExecutionBoth`, strict hold rejection |
| `client_run_test.go` | Add integration coverage for two run use cases sharing one definition |
| `client_hold_test.go` | Add run/hold shared-definition mutual exclusion tests |
| `client_claim_test.go` | Add run/claim shared-definition mutual exclusion tests |
| `examples/sdk/observability-basic/main_test.go` | Preserve public `DefinitionID()` behavior if applicable |

### Existing files to inspect during execution

| File | Why |
|------|-----|
| `lockkit/definitions/types.go` | Confirm `ExecutionBoth`, strict mode fields, composite member expectations |
| `lockkit/runtime/exclusive.go` | Verify sync path assumptions on execution kind and strict mode |
| `lockkit/workers/execute.go` | Verify async path assumptions on execution kind, wait, and idempotency |
| `lockkit/runtime/composite.go` | Likely modify if composite must reuse existing definition IDs during atomic acquire |
| `lockkit/workers/execute_composite.go` | Likely modify or explicitly leave untouched after proving this feature remains sync-only |
| `lockkit/registry/registry.go` | Confirm whether existing registry rules allow composite references to existing shared definitions |

---

## Chunk 1: Shared Definition Core Model

### Task 1: Add failing definition-first API tests

**Files:**
- Create: `definition_test.go`
- Test: `definition_test.go`

- [ ] **Step 1: Add failing definition-first API tests**

Add tests covering:

```go
func TestDefineLockCreatesStableDefinitionID(t *testing.T)
func TestDefineLockRejectsEmptyName(t *testing.T)
func TestDefineLockRejectsMissingBinding(t *testing.T)
func TestDefineRunOnSharesDefinitionAcrossUseCases(t *testing.T)
func TestDefineClaimOnSharesDefinitionAcrossUseCases(t *testing.T)
func TestDefineHoldOnRejectsStrictDefinition(t *testing.T)
func TestRunDefinitionIDRemainsPublicNameFacing(t *testing.T)
func TestDefinitionIDNeverExposesInternalHashedID(t *testing.T)
```

Assertions to include:

```go
contractDef := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))
importUC := DefineRunOn("import", contractDef)
deleteUC := DefineRunOn("delete", contractDef)

if importUC.DefinitionID() != "import" { ... }
if deleteUC.DefinitionID() != "delete" { ... }
if importUC.core.config.definitionRef != deleteUC.core.config.definitionRef { ... }
```

- [ ] **Step 2: Run focused tests to verify failure**

Run: `go test . -run 'TestDefine(Lock|RunOn|ClaimOn|HoldOn)|TestRunDefinitionIDRemainsPublicNameFacing' -v`
Expected: FAIL with undefined `DefineLock`, `DefineRunOn`, `DefineClaimOn`, `DefineHoldOn`, and missing shared-definition config fields.

- [ ] **Step 3: Add `definition.go` with minimal definition model**

Create `definition.go` with:

```go
type DefinitionOption func(*definitionConfig)

type definitionConfig struct {
	strict bool
}

type definitionRef struct {
	name   string
	id     string
	binder any
	config definitionConfig
}

type LockDefinition[T any] struct {
	ref     *definitionRef
	binding Binding[T]
}

func DefineLock[T any](name string, binding Binding[T], opts ...DefinitionOption) LockDefinition[T] { ... }
func stableDefinitionID(name string) string { ... }
func (d LockDefinition[T]) ForceRelease(ctx context.Context, client *Client, resourceKey string) error { ... }
```

Notes:
- Keep `definitionID` internal.
- Make the stable ID name-based only.
- Keep the stored binding typed on `LockDefinition[T]`.
- Implement minimal validation: non-empty name, non-nil binding.

- [ ] **Step 4: Split definition options from use-case options in `binding.go`**

Add:

```go
func Strict() DefinitionOption { ... }
```

Move `strict` out of `useCaseConfig` and into `definitionConfig`.

Keep `UseCaseOption` for:
- `TTL(...)`
- `WaitTimeout(...)`
- `Idempotent()`

Do not add new lineage behavior in this feature work. Preserve existing lineage behavior unless a touched path requires a narrow compatibility adjustment.

- [ ] **Step 5: Add explicit constructor helpers in use case files**

Implement:

```go
func DefineRunOn[T any](name string, def LockDefinition[T], opts ...UseCaseOption) RunUseCase[T] { ... }
func DefineHoldOn[T any](name string, def LockDefinition[T], opts ...UseCaseOption) HoldUseCase[T] { ... }
func DefineClaimOn[T any](name string, def LockDefinition[T], opts ...UseCaseOption) ClaimUseCase[T] { ... }
```

Implementation requirements:
- Existing shorthand constructors remain and internally create an implicit definition.
- `RunUseCase.DefinitionID()` stays public-name-facing to preserve current external behavior.
- `HoldOn` must reject strict definitions immediately.
- Keep startup validation for strict-hold combinations as a defensive second layer.

- [ ] **Step 6: Update `registry.go` core model minimally**

Extend `useCaseCore` and `useCaseConfig` usage so each use case carries a shared `definitionRef` pointer.

Suggested additions:

```go
type useCaseCore struct {
	name       string
	kind       useCaseKind
	config     useCaseConfig
	definition *definitionRef
	registry   *Registry
}
```

Add helper constructors as needed:

```go
func newUseCaseCoreWithDefinition(name string, kind useCaseKind, def *definitionRef, opts ...UseCaseOption) *useCaseCore
```

- [ ] **Step 7: Re-run focused tests**

Run: `go test . -run 'TestDefine(Lock|RunOn|ClaimOn|HoldOn)|TestRunDefinitionIDRemainsPublicNameFacing' -v`
Expected: PASS.

- [ ] **Step 8: Commit core model changes**

```bash
git add definition.go definition_test.go binding.go registry.go usecase_run.go usecase_hold.go usecase_claim.go
git commit -m "feat: add shared lock definition core API"
```

---

### Task 2: Add shorthand compatibility and duplicate definition validation

**Files:**
- Modify: `definition_test.go`
- Modify: `registry.go`
- Modify: `usecase_run.go`
- Modify: `usecase_hold.go`
- Modify: `usecase_claim.go`

- [ ] **Step 1: Add failing shorthand compatibility tests**

Add tests covering:

```go
func TestDefineRunShorthandCreatesImplicitPrivateDefinition(t *testing.T)
func TestRegistryRejectsDuplicateDefinitionNamesWhenExplicitlyRegistered(t *testing.T)
```

Expected behavior:
- Two shorthand use cases with different names do not share a definition.
- Two explicit definitions with the same name cannot both be registered through attached use cases.

- [ ] **Step 2: Run focused tests to verify failure**

Run: `go test . -run 'TestDefineRunShorthandCreatesImplicitPrivateDefinition|TestRegistryRejectsDuplicateDefinitionNamesWhenExplicitlyRegistered' -v`
Expected: FAIL because duplicate shared-definition validation is not yet enforced.

- [ ] **Step 3: Implement implicit definition creation in shorthand constructors**

Use the use case name as the implicit definition name:

```go
func DefineRun[T any](name string, binding Binding[T], opts ...UseCaseOption) RunUseCase[T] {
	def := DefineLock(name, binding)
	return DefineRunOn(name, def, opts...)
}
```

Apply the same pattern to `DefineHold` and `DefineClaim`.

- [ ] **Step 4: Add duplicate-definition validation at planning/registration level**

Implement a helper that collects definition refs from registered use cases and rejects different refs resolving to the same definition name/ID.

Keep this validation tied to registry/client planning, not plain `DefineLock(...)` value construction.

- [ ] **Step 5: Re-run focused tests**

Run: `go test . -run 'TestDefineRunShorthandCreatesImplicitPrivateDefinition|TestRegistryRejectsDuplicateDefinitionNamesWhenExplicitlyRegistered' -v`
Expected: PASS.

- [ ] **Step 6: Commit shorthand compatibility changes**

```bash
git add definition_test.go registry.go usecase_run.go usecase_hold.go usecase_claim.go
git commit -m "feat: preserve shorthand constructors with implicit definitions"
```

---

## Chunk 2: Planner and Engine Normalization

### Task 3: Normalize shared definitions separately from public use cases

**Files:**
- Modify: `client_validation.go`
- Modify: `internal/sdk/usecase.go`
- Modify: `client_run.go`
- Modify: `client_hold.go`
- Modify: `client_claim.go`
- Modify: `client_test.go`
- Modify: `client_run_test.go`
- Modify: `client_hold_test.go`
- Modify: `client_claim_test.go`
- Test: `client_test.go`

- [ ] **Step 1: Add failing client-plan tests for shared-definition registration**

Add tests covering:

```go
func TestNewAllowsMultipleUseCasesToShareOneDefinition(t *testing.T)
func TestSharedDefinitionReferencedByRunAndClaimNormalizesToExecutionBoth(t *testing.T)
func TestHoldOnStrictDefinitionFailsAtStartup(t *testing.T)
func TestRunUsesSharedDefinitionIdentityAtExecutionTime(t *testing.T)
func TestHoldUsesSharedDefinitionIdentityAtExecutionTime(t *testing.T)
func TestClaimUsesSharedDefinitionIdentityAtExecutionTime(t *testing.T)
func TestSharedDefinitionWithHoldAndRunNormalizesToExecutionSync(t *testing.T)
func TestSharedDefinitionWithHoldAndClaimNormalizesToExecutionBoth(t *testing.T)
```

Include assertions that client startup succeeds for run+run sharing one definition and that plan building produces exactly one engine definition identity for the shared definition.

- [ ] **Step 2: Run focused tests to verify failure**

Run: `go test . -run 'TestNewAllowsMultipleUseCasesToShareOneDefinition|TestSharedDefinitionReferencedByRunAndClaimNormalizesToExecutionBoth|TestHoldOnStrictDefinitionFailsAtStartup' -v`
Expected: FAIL because planning still translates one engine definition per use case and uses use-case strictness.

- [ ] **Step 3: Extend `internal/sdk/usecase.go` to accept explicit definition IDs**

Add:

```go
func NewUseCaseWithID(name string, definitionID string, kind UseCaseKind, requirements CapabilityRequirements, link RegistryLink) UseCase
```

Behavior:
- explicit `definitionID` if non-empty
- fallback to current name+kind hashing for old call sites

- [ ] **Step 4: Refactor `normalizeUseCase` to use shared definition IDs**

In `client_validation.go`, replace direct `sdk.NewUseCase(...)` with `sdk.NewUseCaseWithID(...)` using `useCase.definition.id`.

Preserve existing lineage-dependent capability requirements for legacy/non-shared-definition flows unless a touched code path requires a narrow compatibility adjustment.

- [ ] **Step 5: Add shared-definition planning pass**

In `buildClientPlan`, add a pass that:
- gathers unique definition refs from registered use cases
- computes per-definition execution kind:
  - run only => `ExecutionSync`
  - claim only => `ExecutionAsync`
  - run + claim => `ExecutionBoth`
  - hold participation should align with sync handling
- validates strict-definition restrictions for hold

Suggested helper signatures:

```go
type plannedDefinition struct { ... }
func collectPlannedDefinitions(useCases []*useCaseCore) (map[string]plannedDefinition, error)
func executionKindForDefinition(kinds map[useCaseKind]bool) definitions.ExecutionKind
```

- [ ] **Step 6: Register unique engine definitions from planned definitions**

Refactor `registerEngineUseCase` / translation flow so standalone definitions are registered once per shared definition, not once per use case.

Public use cases should still normalize for capability checks and routing, but engine registration must dedupe by shared definition.

- [ ] **Step 7: Update execution paths to use shared definition identity**

Modify `client_run.go`, `client_hold.go`, and `client_claim.go` so runtime execution uses the shared normalized definition identity rather than recomputing per-use-case identities from the old model.

Requirements:
- standalone run requests use shared definition identity
- hold requests use shared definition identity
- claim requests use shared definition identity
- no public API starts exposing internal definition IDs

- [ ] **Step 8: Re-run focused tests**

Run: `go test . -run 'TestNewAllowsMultipleUseCasesToShareOneDefinition|TestSharedDefinitionReferencedByRunAndClaimNormalizesToExecutionBoth|TestHoldOnStrictDefinitionFailsAtStartup|TestRunUsesSharedDefinitionIdentityAtExecutionTime|TestHoldUsesSharedDefinitionIdentityAtExecutionTime|TestClaimUsesSharedDefinitionIdentityAtExecutionTime' -v`
Expected: PASS.

- [ ] **Step 9: Commit planner changes**

```bash
git add client_validation.go internal/sdk/usecase.go client_run.go client_hold.go client_claim.go client_test.go client_run_test.go client_hold_test.go client_claim_test.go
git commit -m "feat: normalize shared lock definitions separately from use cases"
```

---

### Task 4: Map definition-level strictness and per-use-case execution options correctly

**Files:**
- Modify: `client_validation.go`
- Modify: `binding.go`
- Modify: `client_run_test.go`
- Modify: `client_claim_test.go`
- Test: `client_run_test.go`
- Test: `client_claim_test.go`

- [ ] **Step 1: Add failing tests for strict/shared option behavior**

Add tests covering:

```go
func TestSharedStrictDefinitionRequiresStrictBackendSupport(t *testing.T)
func TestSharedClaimUseCaseStillRequiresIdempotencyWhenConfigured(t *testing.T)
```

Before writing implementation in this task, add one explicit design-decision test or assertion documenting the chosen behavior for shared-definition `TTL` and `WaitTimeout`.

Decision for this plan:
- treat `TTL` and `WaitTimeout` as engine-definition-level values once a definition is shared
- require all attached use cases on the same shared definition to agree on non-zero `TTL` and `WaitTimeout`
- reject conflicting values during planning instead of silently harmonizing them

- [ ] **Step 2: Run focused tests to verify failure**

Run: `go test . -run 'TestSharedStrictDefinitionRequiresStrictBackendSupport|TestSharedClaimUseCaseStillRequiresIdempotencyWhenConfigured' -v`
Expected: FAIL because strictness is still modeled on the use case or planner behavior is inconsistent.

- [ ] **Step 3: Add failing conflict-validation tests for shared `TTL` and `WaitTimeout`**

Add tests covering:

```go
func TestSharedDefinitionRejectsConflictingTTLValues(t *testing.T)
func TestSharedDefinitionRejectsConflictingWaitTimeoutValues(t *testing.T)
```

Run: `go test . -run 'TestSharedDefinitionRejectsConflicting(TTLValues|WaitTimeoutValues)' -v`
Expected: FAIL because planning does not yet validate conflicting execution-level settings on shared definitions.

- [ ] **Step 4: Update translation helpers to source strictness from the definition**

In `translateUseCaseDefinition` and related helpers:
- set `ModeStrict` / fencing only from `useCase.definition.config.strict`
- keep claim idempotency tied to the execution surface
- source `LeaseTTL` and `WaitTimeout` for shared definitions from the validated per-definition planned values

- [ ] **Step 5: Add shared-definition option conflict validation in planning**

Extend the planned-definition pass to collect `TTL` and `WaitTimeout` from attached use cases.

Rules:
- all zero values => use defaults
- zero plus one non-zero => use the non-zero value
- multiple distinct non-zero values => reject startup with a clear error naming the definition and conflicting use cases

- [ ] **Step 6: Update capability validation inputs**

Make sure capability checks are driven by actual planned shared-definition requirements:
- strict capability from the shared definition
- idempotency from claim use cases

- [ ] **Step 7: Re-run focused tests**

Run: `go test . -run 'TestSharedStrictDefinitionRequiresStrictBackendSupport|TestSharedClaimUseCaseStillRequiresIdempotencyWhenConfigured|TestSharedDefinitionRejectsConflicting(TTLValues|WaitTimeoutValues)' -v`
Expected: PASS.

- [ ] **Step 8: Commit option-mapping changes**

```bash
git add client_validation.go binding.go client_run_test.go client_claim_test.go
git commit -m "feat: apply strictness at shared definition level"
```

---

## Chunk 3: Composite and Force Release

### Task 5: Replace composite binding reuse with definition-and-projection members

**Files:**
- Modify: `binding.go`
- Modify: `usecase_run.go`
- Modify: `client_validation.go`
- Modify: `lockkit/registry/registry.go`
- Modify: `lockkit/runtime/composite.go`
- Modify: `lockkit/workers/execute_composite.go` only if shared-definition composite support must exist in async paths too
- Modify: `client_test.go`
- Test: `client_test.go`

- [ ] **Step 1: Add failing composite tests**

Add tests covering:

```go
func TestCompositeRunWithSharedDefinitionMembersBuildsProjectedKeys(t *testing.T)
func TestCompositeRunConflictsWithStandaloneUseCaseSharingMemberDefinition(t *testing.T)
func TestCompositeRunRejectsMissingMemberProjection(t *testing.T)
func TestCompositeRunRejectsEmptyMemberName(t *testing.T)
func TestLegacyCompositeOptionStillWorks(t *testing.T)
```

The second test should verify that a composite acquiring `contractDef` conflicts with a standalone `DefineRunOn("import", contractDef)` on the same projected resource key.

- [ ] **Step 2: Run focused tests to verify failure**

Run: `go test . -run 'TestCompositeRunWithSharedDefinitionMembersBuildsProjectedKeys|TestCompositeRunConflictsWithStandaloneUseCaseSharingMemberDefinition' -v`
Expected: FAIL because composite still stores only reused bindings and derives new member IDs from the composite parent.

- [ ] **Step 3: Replace the public composite member API in `binding.go`**

Implement:

```go
type CompositeMember[TInput any] struct {
	name  string
	build func(TInput) (definitionID string, resourceKey string, err error)
}

func Member[TInput any, TMember any](name string, def LockDefinition[TMember], project func(TInput) TMember) CompositeMember[TInput] { ... }
func DefineCompositeRun[T any](name string, members ...CompositeMember[T]) RunUseCase[T] { ... }
```

Requirements:
- `Member(...)` must reuse the definition binding to derive the member resource key.
- Composite member internal shape should be non-generic at storage time.
- **Keep the existing `Composite(...)` option working.** Internally translate old-style `compositeMemberConfig` into the new projection-based representation at planning time. Add deprecation comment on `Composite(...)` and `DefineCompositeMember(...)`.

- [ ] **Step 4: Update composite request binding in `usecase_run.go`**

Replace `buildCompositeMemberInputs` assumptions as needed so each member contributes both the shared definition ID and resource key into request metadata.

- [ ] **Step 5: Update composite planning in `client_validation.go`**

Stop deriving member definition IDs from the composite parent use case.

Instead:
- use the member-provided shared definition ID
- ensure engine registration and composite registration reference existing planned definitions
- keep composite acquire atomic over those existing definitions

- [ ] **Step 6: Update engine composite support, not just client-layer wiring**

Modify the relevant `lockkit` files so composite registration and execution can reference existing shared definition IDs atomically.

At minimum, prove one of these paths with tests and code:
- sync-only composite support for shared definitions, with async composite explicitly left unsupported and guarded
- or sync+async composite support if current workers/composite model can be extended safely

Do not leave this as an “inspect-only” decision.

- No fallback that invents composite-specific member definition IDs is allowed in this plan. If existing `lockkit` structures cannot reference shared definition IDs directly, extend the engine/registry model before exposing the API.

- [ ] **Step 7: Re-run focused tests**

Run: `go test . -run 'TestCompositeRunWithSharedDefinitionMembersBuildsProjectedKeys|TestCompositeRunConflictsWithStandaloneUseCaseSharingMemberDefinition' -v`
Expected: PASS.

- [ ] **Step 8: Commit composite changes**

```bash
git add binding.go usecase_run.go client_validation.go lockkit/registry/registry.go lockkit/runtime/composite.go lockkit/workers/execute_composite.go client_test.go
git commit -m "feat: support shared definitions in composite run use cases"
```

---

### Task 6: Add definition-level force release capability

**Files:**
- Modify: `definition.go`
- Modify: `backend/contracts.go`
- Modify: `backend/redis/driver.go`
- Modify: `definition_test.go`
- Test: `definition_test.go`

- [ ] **Step 1: Add failing force-release tests**

Add tests covering:

```go
func TestForceReleaseRequiresClient(t *testing.T)
func TestForceReleaseRequiresBackendCapability(t *testing.T)
func TestForceReleaseUsesSharedDefinitionID(t *testing.T)
func TestForceReleaseIsIdempotentWhenBackendSupportsIt(t *testing.T)
```

- [ ] **Step 2: Run focused tests to verify failure**

Run: `go test . -run 'TestForceRelease' -v`
Expected: FAIL because the backend capability and method do not exist yet.

- [ ] **Step 3: Add optional backend capability contract**

In `backend/contracts.go`, add:

```go
type ForceReleaseDriver interface {
	ForceReleaseDefinition(ctx context.Context, definitionID, resourceKey string) error
}
```

- [ ] **Step 4: Implement `LockDefinition.ForceRelease` fully**

Behavior:
- nil client => explicit error
- backend missing capability => explicit error
- pass shared internal `definitionID` and resource key

- [ ] **Step 5: Implement Redis force release cleanup**

In `backend/redis/driver.go`, remove:
- primary lease key
- strict fence counter key
- strict token key
- any other auxiliary strict state for the same definition/resource pair

Make the operation idempotent.

- [ ] **Step 6: Re-run focused tests**

Run: `go test . -run 'TestForceRelease' -v`
Expected: PASS.

- [ ] **Step 7: Commit force-release changes**

```bash
git add definition.go backend/contracts.go backend/redis/driver.go definition_test.go
git commit -m "feat: add force release for shared lock definitions"
```

---

## Chunk 4: Docs, Examples, and Full Verification

### Task 7: Update examples and public docs/tests

**Files:**
- Create: `examples/core/shared-lock-definition/main.go`
- Create: `examples/core/shared-lock-definition/main_test.go`
- Modify: `examples/sdk/observability-basic/main_test.go`
- Modify: observability-related root/client tests if shared-definition events need explicit coverage
- Modify: any README/example index files if needed after implementation

- [ ] **Step 1: Add failing example tests**

Add a compile-oriented example test asserting:
- multiple use cases can share a definition
- public `DefinitionID()` remains name-facing for shorthand examples if that API remains exposed
- observability/log-facing behavior continues to use public use case names rather than internal shared definition IDs

- [ ] **Step 2: Run focused example tests to verify failure**

Run: `go test ./examples/... -run 'Test.*Shared.*|TestApproveOrderUseCaseIsDefined' -v`
Expected: FAIL until the new example and any updated name-facing expectations exist.

- [ ] **Step 3: Implement the shared-definition example**

Create a minimal example showing:

```go
contractDef := lockman.DefineLock(...)
importUC := lockman.DefineRunOn("import", contractDef)
holdUC := lockman.DefineHoldOn("manual_hold", contractDef)
```

Keep it small and focused on the shared-definition mental model.

- [ ] **Step 4: Re-run focused example tests**

Run: `go test ./examples/... -run 'Test.*Shared.*|TestApproveOrderUseCaseIsDefined' -v`
Expected: PASS.

- [ ] **Step 5: Commit docs/example changes**

```bash
git add examples/core/shared-lock-definition examples/sdk/observability-basic/main_test.go
git commit -m "docs: add shared lock definition example"
```

---

### Task 8: Run full verification and clean up plan mismatches

**Files:**
- Modify: any touched file as required by verification failures

- [ ] **Step 1: Run package tests for touched root packages**

Run: `go test .`
Expected: PASS.

- [ ] **Step 2: Run full workspace tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 3: Run CI-parity non-workspace tests**

Run: `GOWORK=off go test ./...`
Expected: PASS.

- [ ] **Step 4: Run module-specific suites required by repo guidance**

Run: `go test ./backend/redis/...`
Expected: PASS.

Run: `go test ./idempotency/redis/...`
Expected: PASS.

Run: `go test ./guard/postgres/...`
Expected: PASS.

- [ ] **Step 5: Run tagged example compile checks**

Run: `go test -tags lockman_examples ./examples/... -run '^$'`
Expected: PASS.

- [ ] **Step 6: Run formatting and compile hygiene**

Run: `gofmt -w .`
Expected: files updated only if formatting drift exists.

Run: `go test ./... -run '^$'`
Expected: PASS.

- [ ] **Step 7: Commit final verification fixes**

```bash
git add .
git commit -m "test: verify shared lock definition integration"
```

---

## Implementation Notes

1. Keep the public semantic split sharp:
   - `LockDefinition` owns identity, binding, and strictness.
   - use cases own public naming and execution-specific options.
2. Do not expose the internal hashed `definitionID` through public SDK methods.
3. Prefer adding small helpers in `client_validation.go` over rewriting the whole file at once.
4. If engine limitations force a compromise on per-use-case TTL/wait under shared definitions, document that explicitly and add tests matching the chosen behavior.
5. Do not remove or refactor existing lineage behavior unless a touched codepath requires a narrow compatibility adjustment. Treat lineage as out of scope, not cleanup work.

## Breaking Changes

### Composite member API redesign (Task 5)

The current `Composite[T any](members ...CompositeMember[T]) UseCaseOption` API is replaced by `DefineCompositeRun[T any](name string, members ...CompositeMember[T])`. The existing `CompositeMember[T]` struct shape changes from `{name, binding}` to `{name, build func}`.

**Backward compatibility strategy:** Keep the existing `Composite(...)` option functional. Internally, translate old-style `CompositeMember` configs into the new projection-based representation at planning time. Add a deprecation comment on the old API. Existing composite use cases must continue to work without modification.

Add test:
```go
func TestLegacyCompositeOptionStillWorks(t *testing.T)
```

## DefinitionID() Invariant

`DefinitionID()` on any `RunUseCase`, `ClaimUseCase`, or `HoldUseCase` always returns the **public use case name**, never the internal hashed `definitionID`. This is a preserved invariant for backward compatibility and external observability.

The internal definition ID (e.g., `sdk_def_<hash>`) is used only in:
- engine definition registration
- backend request payloads
- composite member ID derivation

Explicit test to enforce this invariant:
```go
func TestDefinitionIDNeverExposesInternalHashedID(t *testing.T) {
    def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))
    uc := DefineRunOn("import", def)
    // Must be the use case name, not the internal definition hash
    assertEqual(t, uc.DefinitionID(), "import")
    assertNotContains(t, uc.DefinitionID(), "sdk_def_")
}
```

## Execution Kind Resolution for Shared Definitions

When multiple use cases share one definition, the engine execution kind is computed as follows:

| Attached use case kinds | Resulting execution kind |
|------------------------|-------------------------|
| run only | `ExecutionSync` |
| claim only | `ExecutionAsync` |
| hold only | `ExecutionSync` (hold is a sync operation) |
| run + claim | `ExecutionBoth` |
| run + hold | `ExecutionSync` |
| claim + hold | `ExecutionBoth` (claim needs async, hold participates in sync path) |
| run + claim + hold | `ExecutionBoth` |

Add test:
```go
func TestSharedDefinitionWithHoldAndRunNormalizesToExecutionSync(t *testing.T)
func TestSharedDefinitionWithHoldAndClaimNormalizesToExecutionBoth(t *testing.T)
```

## TTL/WaitTimeout Conflict Resolution

Rules for shared definitions (applies per definition, not per use case):

- **All zero values** => use engine defaults
- **One non-zero, rest zero** => use the non-zero value
- **Multiple distinct non-zero values** => reject startup with error naming the definition and conflicting use cases

**Claim vs run semantics note:** `WaitTimeout` remains a declared use-case option in the public API, but once a definition is shared the planner must validate that non-zero `WaitTimeout` values do not conflict across attached use cases. If a shared definition is used by both run and claim and those surfaces specify different non-zero `WaitTimeout` values, reject startup with a clear error.

`TTL` applies uniformly to the lease regardless of execution kind — all attached use cases must agree.

## Force Release Idempotency Semantics

"Idempotent" means: calling `ForceRelease` on a definition/resource pair that has no active lease returns `nil` (no error). The Redis implementation must:
- Use `DEL` or `UNLINK` which are naturally idempotent (return 0 if key doesn't exist)
- Not fail if strict-state keys (fence counter, token) are already absent
- Return a single combined error only if at least one key deletion fails with a non-`not found` error

## Registration Scope Note

Treat SDK registry registration as startup-time configuration, not a concurrent runtime operation. This feature should not add registry locking or expand registry concurrency semantics.

## Suggested Commit Sequence

1. `feat: add shared lock definition core API`
2. `feat: preserve shorthand constructors with implicit definitions`
3. `feat: normalize shared lock definitions separately from use cases`
4. `feat: apply strictness at shared definition level`
5. `feat: support shared definitions in composite run use cases`
6. `feat: add force release for shared lock definitions`
7. `docs: add shared lock definition example`
8. `test: verify shared lock definition integration`

Plan complete and saved to `docs/superpowers/plans/2026-04-03-shared-lock-definition-design.md`. Ready to execute?
