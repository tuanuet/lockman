# Deprecated API Removal Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove deprecated root SDK authoring APIs and align code, tests, examples, benchmarks, consumer fixtures, and docs to the new interface only.

**Architecture:** This is a breaking cleanup pass. The implementation removes deprecated exports from the root package, rewires advanced authoring so strictness lives only on definitions, migrates all in-repo callers to the supported interfaces, and then rewrites docs to teach a single public model. Preserve runtime behavior of the supported interfaces while deleting legacy-only branches.

**Tech Stack:** Go 1.22, root module plus nested modules, Go test, gofmt, GitHub workflow fixture tests, Markdown docs.

---

## File Map

### Production API and runtime files

- Modify: `binding.go`
  Responsibility: remove deprecated root helpers `Strict`, `DefineCompositeMember`, and `Composite`; keep only supported composite primitives needed by current runtime.
- Modify: `usecase_run.go`
  Responsibility: remove deprecated shorthand `DefineRun`; preserve `DefineRunOn` request-building behavior.
- Modify: `usecase_hold.go`
  Responsibility: remove deprecated shorthand `DefineHold`; preserve `DefineHoldOn` behavior.
- Modify: `usecase_claim.go`
  Responsibility: remove deprecated shorthand `DefineClaim`; preserve `DefineClaimOn` behavior.
- Modify: `advanced/strict/api.go`
  Responsibility: remove deprecated strict authoring wrapper or repoint the package so it no longer depends on removed `lockman.Strict()`.
- Modify: `advanced/strict/doc.go`
  Responsibility: keep package docs aligned with the new strict-definition-only story if the package remains.
- Modify: `advanced/composite/api.go`
  Responsibility: ensure advanced composite no longer depends on removed deprecated root helpers.
- Modify: `advanced/composite/doc.go`
  Responsibility: keep package docs aligned with supported composite authoring.

### Root tests and advanced-package tests

- Modify: `definition_test.go`
  Responsibility: remove shorthand constructor coverage; keep supported definition-sharing coverage.
- Modify: `client_test.go`
  Responsibility: replace deprecated strict/composite/shorthand callsites with supported setup.
- Modify: `advanced/strict/api_test.go`
  Responsibility: rewrite or remove tests to match the strict-definition-only public story.
- Modify: `advanced/composite/api_test.go`
  Responsibility: rewrite strict composite coverage to definition-level strictness semantics.
- Modify: any other root tests surfaced by grep during implementation.

### Benchmarks and repo-owned consumer fixtures

- Modify: `benchmarks/benchmark_adoption_baseline_test.go`
- Modify: `benchmarks/benchmark_adoption_adapter_test.go`
- Modify: `benchmarks/benchmark_redislock_test.go` if needed
- Modify: `benchmarks/benchmark_adoption_helpers_test.go` if helper constructors encode removed APIs
- Modify: `testdata/externalconsumer/smoke_test.go`
- Modify: `testdata/releaseconsumer/root_smoke_test.go`
- Modify: `external_consumer_surface_test.go`
- Modify: `release_workflow_surface_test.go` only if workflow assertions must change to track the updated consumer surface

### Canonical docs and runnable examples

- Modify: `README.md`
- Modify: `CHANGELOG.md`
- Modify: `docs/registry-and-usecases.md`
- Modify: `docs/lock-definition-reference.md`
- Modify: `docs/production-guide.md`
- Modify: `docs/advanced/strict.md`
- Modify: `docs/advanced/composite.md`
- Modify: `docs/quickstart-sync.md` if removed APIs appear
- Modify: `docs/quickstart-async.md` if removed APIs appear
- Modify: affected `examples/sdk/**/README.md`
- Modify: affected `examples/sdk/**/*.go`
- Modify: affected adapter example READMEs or `main.go` files

### Reference inputs for implementation

- Read: `docs/superpowers/specs/2026-04-03-deprecated-api-removal-design.md`
- Read: `docs/superpowers/specs/2026-04-03-advanced-interface-design.md`
- Read: `AGENTS.md`

## Chunk 1: Remove Deprecated Root Exports

### Task 1: Lock in the first failing tests for removed shorthand constructors

**Files:**
- Modify: `definition_test.go`
- Test: `definition_test.go`

- [ ] **Step 1: Write the failing test updates**

Replace shorthand-focused tests with supported-interface expectations. Remove tests named around implicit private definitions and add a replacement test that proves explicit definitions remain the only supported authoring path.

Target replacement test shape:

```go
func TestDefineRunOnRequiresExplicitDefinitionSharing(t *testing.T) {
	defA := DefineLock("order.create", BindResourceID("order", func(v string) string { return v }))
	defB := DefineLock("order.delete", BindResourceID("order", func(v string) string { return v }))

	ucA := DefineRunOn("order.create", defA)
	ucB := DefineRunOn("order.delete", defB)

	if ucA.core.config.definitionRef == nil || ucB.core.config.definitionRef == nil {
		t.Fatal("expected explicit definitions to be attached")
	}
	if ucA.core.config.definitionRef == ucB.core.config.definitionRef {
		t.Fatal("expected independently defined use cases to keep separate definitions")
	}
}
```

Also add a compile-only assertion that the removed shorthand names must not exist anymore once production cleanup lands:

```go
// This file should stop compiling until DefineRun/DefineHold/DefineClaim are removed.
var _ = DefineRun[string]
var _ = DefineHold[string]
var _ = DefineClaim[string]
```

- [ ] **Step 2: Run the targeted test to verify RED**

Run: `go test . -run '^TestDefineRunOnRequiresExplicitDefinitionSharing$' -v`
Expected: FAIL to compile because the temporary compile-only assertion still references `DefineRun`, `DefineHold`, and `DefineClaim` until those exports are removed in Step 3.

- [ ] **Step 3: Remove the deprecated shorthand exports**

Delete:

```go
func DefineRun[T any](name string, binding Binding[T], opts ...UseCaseOption) RunUseCase[T]
func DefineHold[T any](name string, binding Binding[T], opts ...UseCaseOption) HoldUseCase[T]
func DefineClaim[T any](name string, binding Binding[T], opts ...UseCaseOption) ClaimUseCase[T]
```

from:

- `usecase_run.go`
- `usecase_hold.go`
- `usecase_claim.go`

Do not change `DefineRunOn`, `DefineHoldOn`, or `DefineClaimOn` behavior beyond compile fallout from the removals.

- [ ] **Step 4: Run the targeted test to verify GREEN**

Run: `go test . -run 'TestDefineRunOnSharesDefinitionAcrossUseCases|TestDefineClaimOnSharesDefinitionAcrossUseCases|TestDefineHoldOnRejectsStrictDefinition|TestDefineRunOnRequiresExplicitDefinitionSharing' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add definition_test.go usecase_run.go usecase_hold.go usecase_claim.go
git commit -m "refactor: remove shorthand use case constructors"
```

### Task 2: Remove deprecated strict/composite root helpers

**Files:**
- Modify: `binding.go`
- Modify: `client_test.go`
- Modify: `advanced/strict/api.go`
- Modify: `advanced/strict/api_test.go`
- Modify: `advanced/composite/api_test.go`
- Test: `client_test.go`

- [ ] **Step 1: Write the failing test updates**

Rewrite deprecated-helper tests to supported shapes.

Replace strict setup like:

```go
uc := DefineRun[string]("order.strict", BindResourceID("order", func(v string) string { return v }), Strict())
```

with:

```go
strictDef := DefineLock("order.strict", BindResourceID("order", func(v string) string { return v }), StrictDef())
uc := DefineRunOn("order.strict", strictDef)
```

Replace deprecated composite setup like:

```go
transferUC := DefineRun(
	"transfer.run",
	BindKey(func(in compositeOrderInput) string { return in.OrderID }),
	Composite(
		DefineCompositeMember("primary", BindResourceID("order", func(in compositeOrderInput) string { return in.OrderID })),
	),
)
```

with:

```go
accountDef := DefineLock("account", BindResourceID("account", func(in compositeOrderInput) string { return in.OrderID }))
ledgerDef := DefineLock("ledger", BindResourceID("ledger", func(in compositeOrderInput) string { return in.OrderID + "-sec" }))
transferDef := composite.DefineLock("transfer", accountDef, ledgerDef)
transferUC := composite.AttachRun("transfer.run", transferDef)
```

- [ ] **Step 2: Run the targeted tests to verify RED**

Run: `go test . -run 'TestNewFailsWhenStrictUseCaseNeedsStrictBackendSupport|TestNewFailsWhenHoldUseCaseUsesStrictMode|TestLegacyCompositeOptionStillWorks|TestCompositeRunWithSharedDefinitionMembersBuildsProjectedKeys|TestCompositeRunRejectsMissingMemberProjection|TestCompositeRunRejectsEmptyMemberName' -v`
Expected: FAIL after the test updates because production code still exports and depends on the deprecated helper path.

- [ ] **Step 3: Remove the deprecated root helpers**

Delete from `binding.go`:

```go
func Strict() UseCaseOption
func DefineCompositeMember[T any](name string, binding Binding[T]) CompositeMember[T]
func Composite[T any](members ...CompositeMember[T]) UseCaseOption
```

Then remove any legacy-only branches whose only purpose was supporting those helpers.

In the same step, clear the immediate compile fallout so this chunk remains buildable:

- remove `lockman.Strict()` usage from `advanced/strict/api.go`
- migrate `advanced/strict/api_test.go` to the strict-definition path or package-removal expectations
- migrate `advanced/composite/api_test.go` off `lockman.Strict()` and onto definition-level strictness

- [ ] **Step 4: Run the targeted tests to verify GREEN**

Run: `go test . -run 'TestNewFailsWhenStrictUseCaseNeedsStrictBackendSupport|TestNewFailsWhenHoldUseCaseUsesStrictMode|TestCompositeRunWithSharedDefinitionMembersBuildsProjectedKeys|TestCompositeRunRejectsMissingMemberProjection|TestCompositeRunRejectsEmptyMemberName' -v && go test ./advanced/strict ./advanced/composite -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add binding.go client_test.go advanced/strict/api.go advanced/strict/api_test.go advanced/composite/api_test.go
git commit -m "refactor: remove deprecated strict and composite helpers"
```

## Chunk 2: Rework Advanced Authoring Around Supported APIs

### Task 3: Remove advanced strict wrapper authoring

**Files:**
- Modify: `advanced/strict/api.go`
- Modify: `advanced/strict/api_test.go`
- Modify: `advanced/strict/doc.go`
- Modify: `docs/advanced/strict.md`
- Modify: `examples/sdk/sync-fenced-write/main.go`
- Modify: `examples/sdk/sync-fenced-write/main_test.go`
- Modify: `examples/core/sync-fenced-write/main.go`
- Modify: `examples/core/sync-fenced-write/main_test.go`
- Modify: `backend/redis/examples/sync-fenced-write/main.go`
- Modify: `benchmarks/benchmark_adoption_baseline_test.go`
- Modify: `benchmarks/benchmark_adoption_adapter_test.go`
- Test: `advanced/strict/api_test.go`

- [ ] **Step 1: Write the failing test updates**

Replace the old wrapper test with root strict-definition coverage or package-level removal coverage.

If the package is retained only as historical/module doc surface, the test should verify the supported strict path directly from root APIs:

```go
strictDef := lockman.DefineLock(
	"order.strict-write",
	lockman.BindResourceID("order", func(v string) string { return v }),
	lockman.StrictDef(),
)
approve := lockman.DefineRunOn("order.strict-approve", strictDef)
```

- [ ] **Step 2: Run the targeted tests to verify RED**

Run: `go test ./advanced/strict ./benchmarks ./examples/sdk/sync-fenced-write ./examples/core/sync-fenced-write ./backend/redis/examples/sync-fenced-write -run TestStrictPackageExposesPublicRunUseCaseAuthoring -v`
Expected: FAIL because in-repo strict authoring still depends on the wrapper path.

- [ ] **Step 3: Apply the minimal implementation change**

Implement the chosen breaking outcome from the spec:

- remove `advanced/strict.DefineRunOn(...)`
- keep the package only as doc/package-level historical surface if needed, but not as a public authoring wrapper

Then update the doc surface to tell users to use:

```go
strictDef := lockman.DefineLock(..., lockman.StrictDef())
approve := lockman.DefineRunOn("...", strictDef)
```

In the same step, migrate every in-repo strict authoring callsite listed in the file scope to the root strict-definition path so the repository does not retain broken references to the removed wrapper.

- [ ] **Step 4: Run the targeted tests to verify GREEN**

Run: `go test ./advanced/strict ./benchmarks ./examples/sdk/sync-fenced-write ./examples/core/sync-fenced-write ./backend/redis/examples/sync-fenced-write -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add advanced/strict/api.go advanced/strict/api_test.go advanced/strict/doc.go docs/advanced/strict.md examples/sdk/sync-fenced-write examples/core/sync-fenced-write backend/redis/examples/sync-fenced-write benchmarks/benchmark_adoption_baseline_test.go benchmarks/benchmark_adoption_adapter_test.go
git commit -m "refactor: remove advanced strict wrapper authoring"
```

### Task 4: Keep advanced composite on the supported composite path only

**Files:**
- Modify: `advanced/composite/api.go`
- Modify: `advanced/composite/api_test.go`
- Modify: `advanced/composite/doc.go`
- Modify: `docs/advanced/composite.md`
- Test: `advanced/composite/api_test.go`

- [ ] **Step 1: Write the failing test updates**

Rewrite strict composite coverage so it no longer passes `lockman.Strict()` into `AttachRun(...)`.

Test through the supported definition-level strictness model. If the implementation continues rejecting strict composite runs, verify that through a strict child definition. If the implementation supports a stricter composite definition shape instead, verify that supported path directly. Choose the path the implementation actually supports and keep docs aligned with it.

```go
strictAccountDef := lockman.DefineLock(
	"account",
	lockman.BindResourceID("account", func(in transferInput) string { return in.AccountID }),
	lockman.StrictDef(),
)
```

- [ ] **Step 2: Run the targeted test to verify RED**

Run: `go test ./advanced/composite -run TestCompositePackageRejectsStrictCompositeRuns -v`
Expected: FAIL until the test and implementation are aligned to definition-level strictness.

- [ ] **Step 3: Apply the minimal implementation change**

Keep `DefineLock(...)` and `AttachRun(...)` as the advanced composite public path, but ensure neither depends on removed root helpers. Update tests and docs to show only supported composite authoring.

- [ ] **Step 4: Run the targeted test to verify GREEN**

Run: `go test ./advanced/composite -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add advanced/composite/api.go advanced/composite/api_test.go advanced/composite/doc.go docs/advanced/composite.md
git commit -m "refactor: align advanced composite with supported strict model"
```

## Chunk 3: Migrate Benchmarks, Fixtures, and Root Tests

### Task 5: Rewrite remaining root tests away from removed APIs

**Files:**
- Modify: `client_test.go`
- Modify: `definition_test.go`
- Test: root package tests

- [ ] **Step 1: Write the failing test updates**

Delete or replace every test whose subject is deprecated compatibility.

Examples to remove or rename:

- `TestDefineRunShorthandCreatesImplicitPrivateDefinition`
- `TestDefineHoldShorthandCreatesImplicitPrivateDefinition`
- `TestDefineClaimShorthandCreatesImplicitPrivateDefinition`
- `TestLegacyCompositeOptionStillWorks`

Replace with behavior-focused tests under the supported interfaces.

- [ ] **Step 2: Run the targeted package tests to verify RED**

Run: `go test . -run 'TestDefine|TestNew' -v`
Expected: FAIL from compile or expectation mismatch until all removed API callsites are migrated.

- [ ] **Step 3: Apply the minimal implementation change**

Update remaining root tests to only call:

- `DefineLock`
- `StrictDef`
- `DefineRunOn`
- `DefineHoldOn`
- `DefineClaimOn`
- `advanced/composite.DefineLock`
- `advanced/composite.AttachRun`

- [ ] **Step 4: Run the targeted package tests to verify GREEN**

Run: `go test . -run 'TestDefine|TestNew' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client_test.go definition_test.go
git commit -m "test: migrate root package coverage to new APIs"
```

### Task 6: Rewrite benchmarks and consumer fixtures

**Files:**
- Modify: `benchmarks/benchmark_adoption_baseline_test.go`
- Modify: `benchmarks/benchmark_adoption_adapter_test.go`
- Modify: `benchmarks/benchmark_adoption_helpers_test.go` if shared setup uses removed APIs
- Modify: `testdata/externalconsumer/smoke_test.go`
- Modify: `testdata/releaseconsumer/root_smoke_test.go`
- Modify: `external_consumer_surface_test.go`
- Modify: `release_workflow_surface_test.go` if assertions need fixture-name updates only
- Test: targeted benchmark compile checks and consumer-surface tests

- [ ] **Step 1: Write the failing fixture updates**

Convert fixture and benchmark authoring snippets to explicit definitions.

Example replacement pattern:

```go
orderDef := lockman.DefineLock(
	"order",
	lockman.BindResourceID("order", func(in Input) string { return in.OrderID }),
)
approve := lockman.DefineRunOn("order.approve", orderDef)
```

- [ ] **Step 2: Run the targeted checks to verify RED**

Run: `go test ./benchmarks -run '^$' && GOWORK=off go test . -run 'TestCIWorkflowCoversExternalConsumerInstall|TestExternalConsumerSmokeFixtureImportsReleasedModules|TestReleaseWorkflow' -v`
Expected: FAIL because benchmark snippets or repo-owned consumer fixtures still encode removed APIs.

- [ ] **Step 3: Apply the minimal implementation change**

Rewrite repo-owned benchmark snippets and consumer fixtures so they compile and teach only the supported interfaces.

- [ ] **Step 4: Run the targeted checks to verify GREEN**

Run: `go test ./benchmarks -run '^$' && GOWORK=off go test . -run 'TestCIWorkflowCoversExternalConsumerInstall|TestExternalConsumerSmokeFixtureImportsReleasedModules|TestReleaseWorkflow' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add benchmarks testdata external_consumer_surface_test.go release_workflow_surface_test.go
git commit -m "test: update benchmarks and consumer fixtures for new APIs"
```

## Chunk 4: Rewrite Docs, Examples, and Release Framing

### Task 7: Rewrite runnable examples before doc narrative cleanup

**Files:**
- Modify: `examples/sdk/sync-approve-order/main.go`
- Modify: `examples/sdk/sync-approve-order/main_test.go`
- Modify: `examples/sdk/async-process-order/main.go`
- Modify: `examples/sdk/async-process-order/main_test.go`
- Modify: `examples/sdk/shared-lock-definition/main.go`
- Modify: `examples/sdk/shared-lock-definition/main_test.go`
- Modify: `examples/sdk/shared-aggregate-split-definitions/main.go`
- Modify: `examples/sdk/shared-aggregate-split-definitions/main_test.go`
- Modify: `examples/sdk/parent-lock-over-composite/main.go`
- Modify: `examples/sdk/parent-lock-over-composite/main_test.go`
- Modify: `examples/sdk/sync-transfer-funds/main.go`
- Modify: `examples/sdk/sync-transfer-funds/main_test.go`
- Modify: `examples/sdk/observability-basic/main.go`
- Modify: `examples/sdk/observability-basic/main_test.go`
- Modify: `examples/sdk/sync-fenced-write/main.go`
- Modify: `examples/sdk/sync-fenced-write/main_test.go`
- Modify: `examples/sdk/sync-approve-order/README.md`
- Modify: `examples/sdk/async-process-order/README.md`
- Modify: `examples/sdk/shared-lock-definition/README.md`
- Modify: `examples/sdk/shared-aggregate-split-definitions/README.md`
- Modify: `examples/sdk/parent-lock-over-composite/README.md`
- Modify: `examples/sdk/sync-transfer-funds/README.md`
- Modify: `examples/sdk/observability-basic/README.md`
- Modify: `examples/sdk/sync-fenced-write/README.md`
- Modify: `examples/README.md`
- Modify: `backend/redis/examples/sync-approve-order/main.go`
- Modify: `backend/redis/examples/sync-approve-order/README.md`
- Modify: `backend/redis/examples/sync-transfer-funds/main.go`
- Modify: `backend/redis/examples/sync-transfer-funds/README.md`
- Modify: `backend/redis/examples/sync-fenced-write/main.go`
- Modify: `backend/redis/examples/sync-fenced-write/README.md`
- Modify: `idempotency/redis/examples/async-process-order/README.md`
- Test: tagged example compile

- [ ] **Step 1: Write the failing example updates**

Update source examples that still use removed APIs. Keep examples minimal and aligned to the current public path.

Key migrations:

- sync/hold/claim examples use `DefineLock + ...On`
- strict examples use `DefineLock(..., StrictDef()) + DefineRunOn`
- composite examples use `advanced/composite.DefineLock + AttachRun` or supported root internals only where they are not the public teaching path

- [ ] **Step 2: Run the tagged example compile to verify RED**

Run: `go test -tags lockman_examples ./examples/... -run '^$' && go test ./backend/redis/examples/... -run '^$' && go test ./idempotency/redis/examples/... -run '^$'`
Expected: FAIL until all example callsites are migrated.

- [ ] **Step 3: Apply the minimal implementation change**

Rewrite all affected example source and README snippets to the supported interfaces only.

- [ ] **Step 4: Run the tagged example compile to verify GREEN**

Run: `go test -tags lockman_examples ./examples/... -run '^$' && go test ./backend/redis/examples/... -run '^$' && go test ./idempotency/redis/examples/... -run '^$'`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add examples backend/redis/examples idempotency/redis/examples
git commit -m "docs: migrate examples to supported APIs"
```

### Task 8: Rewrite canonical docs and changelog

**Files:**
- Modify: `README.md`
- Modify: `CHANGELOG.md`
- Modify: `docs/registry-and-usecases.md`
- Modify: `docs/lock-definition-reference.md`
- Modify: `docs/production-guide.md`
- Modify: `docs/advanced/strict.md`
- Modify: `docs/advanced/composite.md`
- Modify: `docs/advanced/guard.md`
- Modify: `docs/quickstart-sync.md`
- Modify: `docs/quickstart-async.md`
- Test: doc surface tests and repository search

- [ ] **Step 1: Write the failing doc updates**

Delete deprecation-era wording and replace it with current-state wording.

Required direction:

- no `deprecated but still functional`
- no `next major will remove`
- no migration snippets that imply removed APIs still exist
- strict docs teach `StrictDef()`
- composite docs teach the advanced composite path

- [ ] **Step 2: Run the targeted doc checks to verify RED**

Run: `make test-docs`
Expected: FAIL if adoption docs or doc-linked tests still encode stale API framing.

Then run repository searches:

1. `rg -n 'DefineRun\(|DefineHold\(|DefineClaim\(' README.md docs examples backend/redis/examples idempotency/redis/examples --glob '!docs/superpowers/**'`
2. `rg -n 'Strict\(\)|DefineCompositeMember\(' README.md docs examples backend/redis/examples idempotency/redis/examples --glob '!docs/superpowers/**'`

Expected: those searches still show stale canonical references before the rewrite.

- [ ] **Step 3: Apply the minimal implementation change**

Rewrite the canonical Markdown files so they describe one supported public story only.

Update `CHANGELOG.md` unreleased notes to say the deprecated APIs were removed in the current line.

- [ ] **Step 4: Run the targeted doc checks to verify GREEN**

Run: `make test-docs`
Expected: PASS.

Then run repository searches again:

1. `rg -n 'DefineRun\(|DefineHold\(|DefineClaim\(' README.md docs examples backend/redis/examples idempotency/redis/examples --glob '!docs/superpowers/**'`
2. `rg -n 'Strict\(\)|DefineCompositeMember\(' README.md docs examples backend/redis/examples idempotency/redis/examples --glob '!docs/superpowers/**'`

Expected: no matches in canonical user-facing docs and example READMEs except in historical spec/plan files under `docs/superpowers/`.

- [ ] **Step 5: Commit**

```bash
git add README.md CHANGELOG.md docs
git commit -m "docs: remove deprecated API guidance"
```

## Chunk 5: Final Verification and Cleanup

### Task 9: Run formatting and repository-wide verification

**Files:**
- Modify: all Go files touched in previous chunks
- Test: full repository verification

- [ ] **Step 1: Run formatting**

Run: `gofmt -w binding.go usecase_run.go usecase_hold.go usecase_claim.go advanced/strict/api.go advanced/strict/api_test.go advanced/composite/api.go advanced/composite/api_test.go client_test.go definition_test.go benchmarks/benchmark_adoption_baseline_test.go benchmarks/benchmark_adoption_adapter_test.go benchmarks/benchmark_adoption_helpers_test.go examples/sdk/sync-approve-order/main.go examples/sdk/sync-approve-order/main_test.go examples/sdk/async-process-order/main.go examples/sdk/async-process-order/main_test.go examples/sdk/shared-lock-definition/main.go examples/sdk/shared-lock-definition/main_test.go examples/sdk/shared-aggregate-split-definitions/main.go examples/sdk/shared-aggregate-split-definitions/main_test.go examples/sdk/parent-lock-over-composite/main.go examples/sdk/parent-lock-over-composite/main_test.go examples/sdk/sync-transfer-funds/main.go examples/sdk/sync-transfer-funds/main_test.go examples/sdk/observability-basic/main.go examples/sdk/observability-basic/main_test.go examples/sdk/sync-fenced-write/main.go examples/sdk/sync-fenced-write/main_test.go examples/core/sync-fenced-write/main.go examples/core/sync-fenced-write/main_test.go backend/redis/examples/sync-approve-order/main.go backend/redis/examples/sync-transfer-funds/main.go backend/redis/examples/sync-fenced-write/main.go idempotency/redis/examples/async-process-order/main.go testdata/externalconsumer/smoke_test.go testdata/releaseconsumer/root_smoke_test.go external_consumer_surface_test.go release_workflow_surface_test.go`
Expected: touched Go files are formatted.

- [ ] **Step 2: Run focused symbol searches**

Run these commands:

1. `rg -n 'DefineRun\(|DefineHold\(|DefineClaim\(' . --glob '!docs/superpowers/**'`
2. `rg -n 'Strict\(\)|DefineCompositeMember\(' . --glob '!docs/superpowers/**'`

Expected: no matches in canonical production code, examples, benchmarks, fixtures, or user-facing docs that imply those APIs still exist.

- [ ] **Step 3: Run full verification**

Run each command from repo root:

1. `go test ./...`
2. `GOWORK=off go test ./...`
3. `go test ./backend/redis/...`
4. `go test ./idempotency/redis/...`
5. `go test ./guard/postgres/...`
6. `go test -tags lockman_examples ./examples/... -run '^$'`
7. `GOWORK=off go test . -run 'TestCIWorkflowCoversExternalConsumerInstall|TestExternalConsumerSmokeFixtureImportsReleasedModules'`
8. `GOWORK=off go test . -run '^TestReleaseWorkflow'`

Expected: all commands pass.

- [ ] **Step 4: Commit final formatting or cleanup**

```bash
git add .
git commit -m "chore: finalize deprecated API removal"
```

## Notes For The Implementer

- Follow TDD literally: update the smallest relevant test first, watch it fail, then make the smallest code change to restore green.
- Do not reintroduce compatibility wrappers after removing deprecated APIs.
- Keep runtime semantics stable for supported interfaces.
- If compile fallout reveals an overlooked removed-API callsite, migrate it in the chunk where it logically belongs, then re-run the nearest focused test before continuing.
- Keep commits scoped to the chunk they complete.
