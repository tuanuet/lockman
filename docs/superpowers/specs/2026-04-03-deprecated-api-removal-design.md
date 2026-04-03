# Deprecated API Removal Design

## Context

The repository currently exposes two groups of public APIs that are already marked deprecated:

1. Root shorthand use case constructors:
   - `DefineRun`
   - `DefineHold`
   - `DefineClaim`
2. Root advanced helper surfaces:
   - `Strict`
   - `DefineCompositeMember`
   - `Composite`

The repo has already shifted its public SDK story to the definition-first model built around `DefineLock`, `StrictDef`, `DefineRunOn`, `DefineHoldOn`, `DefineClaimOn`, and the newer advanced composite interface.

The remaining deprecated surfaces keep backward-compatibility paths alive in public code, tests, examples, and docs. The user explicitly wants those deprecated parts removed entirely and wants both code and docs aligned to the new interface only.

## Goal

Remove all deprecated public API surfaces from the current line and align the repository to one public authoring model:

1. define a lock boundary with `DefineLock`
2. apply strictness at the definition level with `StrictDef` when needed
3. attach execution surfaces with `DefineRunOn`, `DefineHoldOn`, or `DefineClaimOn`
4. model composite runs through the definition-first advanced composite path, centered on `advanced/composite.DefineLock(...)` plus `advanced/composite.AttachRun(...)`

This is an intentional breaking change.

## Non-Goals

- redesigning the runtime, backend contracts, or registry model
- changing the behavior of the new interface
- introducing compatibility shims, aliases, or adapter layers for the removed APIs
- broad unrelated refactors outside the deprecated-surface cleanup

## Recommended Approach

Use a full breaking cleanup pass.

That means:

1. remove deprecated exports from the root package
2. rewrite internal tests to exercise only the replacement interfaces
3. rewrite examples so all runnable source uses the new interface
4. rewrite docs so the repository teaches only the new model

This is the cleanest match for the requested outcome. It avoids a half-migrated state where the repo message is new but the public surface and fixtures still preserve the old path.

## Alternatives Considered

### Option A: Remove public exports but keep compatibility-only tests or fixtures

This preserves some migration archaeology, but it leaves the repository carrying old conceptual weight. It also increases the chance that future maintainers continue optimizing for removed APIs.

### Option B: Update docs and examples first, then remove code later

This lowers short-term risk but leaves the repository in an inconsistent state. It does not satisfy the explicit request to remove deprecated parts now.

## Design

### 1. Public API Surface

Remove these deprecated exports from the root package:

- `DefineRun`
- `DefineHold`
- `DefineClaim`
- `Strict`
- `DefineCompositeMember`
- `Composite`

Keep these public entry points as the canonical interface:

- `DefineLock`
- `StrictDef`
- `DefineRunOn`
- `DefineHoldOn`
- `DefineClaimOn`

For composite authoring, the canonical public end state for this cleanup should follow the repository's current supported advanced path:

- `advanced/composite.DefineLock(...)`
- `advanced/composite.AttachRun(...)`

Root `Member`, `DefineCompositeRun`, and `DefineCompositeRunWithOptions` may remain as implementation primitives if still needed internally or for package layering, but they are not the public documentation target for this cleanup.

The end state is that the root package exposes only the definition-first path for these concerns.

### 2. Production Code Changes

#### 2.1 Use case constructors

Delete the shorthand constructor functions from:

- `usecase_run.go`
- `usecase_hold.go`
- `usecase_claim.go`

After removal, the package should only expose `DefineRunOn`, `DefineHoldOn`, and `DefineClaimOn` for attaching execution surfaces.

#### 2.2 Deprecated strict/composite helpers

Delete deprecated helper functions from `binding.go`:

- `Strict`
- `DefineCompositeMember`
- `Composite`

Keep the newer composite plumbing in place as needed to support the advanced composite public path. The intended public-facing migration target is the advanced package's definition-first composite shape, not the removed root helpers.

#### 2.3 Legacy support logic

Remove internal logic whose only purpose is to support deprecated authoring paths.

This includes:

- implicit definition creation performed by shorthand constructors
- legacy composite option plumbing driven by `Composite(...)`
- strict configuration plumbing driven by `Strict()`

The exact code deletions should remain minimal, but the public behavior after cleanup should have no dependency on the removed APIs.

### 3. Tests

Rewrite tests that currently exercise deprecated constructors or options.

The replacement tests should cover the same real behaviors through the new public interface:

- shared definition identity and use case attachment
- strict-definition validation and behavior
- composite member binding and ordering
- registry validation for shared definitions and composites
- client behavior for run, hold, and claim requests built from `...On` helpers

Delete tests whose only purpose is to verify backward compatibility for removed APIs.

### 4. Examples

All runnable examples under the canonical SDK and adapter-backed paths should use the new interface directly.

Examples must no longer include source code based on:

- `DefineRun`
- `DefineHold`
- `DefineClaim`
- `Strict`
- `DefineCompositeMember`
- `Composite`

If an example currently demonstrates an old shape, it should be rewritten to the equivalent new shape rather than retained as legacy coverage.

This scope includes:

- root `examples/...`
- nested-module adapter examples
- benchmark fixtures when they still use removed APIs
- repo-owned external-consumer smoke fixtures under `testdata/...`

### 5. Documentation

Rewrite canonical docs to teach a single model and remove deprecation framing that is now stale.

Required documentation changes:

- `README.md`
- `docs/advanced/strict.md`
- `docs/advanced/composite.md`
- `docs/registry-and-usecases.md`
- `docs/lock-definition-reference.md`
- `docs/production-guide.md`
- any quickstart or example README that still references removed APIs
- changelog or release notes, if they still describe these APIs as merely deprecated

Composite docs should align with the approved advanced-interface direction. That means the canonical documented composite path should be the advanced package's definition-first interface, not the removed root deprecated helpers.

Required wording changes:

- remove phrases like `deprecated but still functional`
- remove migration sections that describe the old API as present in the current line
- replace `next major will remove` framing with direct statements that the current API is definition-first and the old surface is gone

Docs should explain only the current interface.

### 6. Advanced Packages

The repository already has explicit advanced package entry points such as:

- `github.com/tuanuet/lockman/advanced/strict`
- `github.com/tuanuet/lockman/advanced/composite`

These should remain aligned to the new interface and should not reintroduce deprecated root helpers indirectly.

For composite specifically, the cleanup should preserve and reinforce the repository's approved direction:

1. child parts are authored as real root `lockman.LockDefinition[T]` values
2. the advanced composite package is the public teaching surface for composite definition-first authoring
3. the existing `DefineLock` plus `AttachRun` shape is the supported public path for this change

#### 6.1 Advanced strict package

`advanced/strict` currently depends on the deprecated root `lockman.Strict()` option.

That dependency must be removed as part of this cleanup.

The end state for this change is explicit:

1. strictness is definition-level only
2. strict authoring is done with `lockman.DefineLock(..., lockman.StrictDef())`
3. strict runs attach through the normal root path `lockman.DefineRunOn(...)`
4. `advanced/strict` is no longer a canonical public authoring surface
5. `advanced/strict.DefineRunOn(...)` should be removed in the same breaking pass rather than reimplemented through new hidden coupling

This means `docs/advanced/strict.md`, examples, and tests should move to the root strict-definition model directly.

#### 6.2 Advanced composite strict behavior

Current advanced composite tests express `strict composite` through `AttachRun(..., lockman.Strict())`.

That path disappears with `lockman.Strict()`.

The replacement rule is:

1. strictness, if represented at all, must originate from definitions rather than use case options
2. composite strict validation should be exercised through strict child definitions or a strict composite definition shape, whichever the implementation supports
3. no advanced composite public API may continue to depend on the removed `Strict()` option

If strict composite runs remain unsupported, tests should verify rejection through the new definition-level authoring path rather than through a removed option.

#### 6.3 Root support needed by advanced packages

Because `advanced/composite` currently relies on deprecated root hooks, this cleanup must explicitly migrate those dependencies.

Allowed outcomes are:

1. add or expose minimal root helpers needed for the advanced packages to stay on the new model
2. narrow advanced package behavior where that matches the new definition-level rules
3. change advanced package APIs in the same breaking pass

For `advanced/strict`, that choice is already made above: remove the authoring wrapper and teach the root strict-definition path directly.

For `advanced/composite`, the implementation must choose one of the allowed outcomes explicitly. Leaving advanced packages implicitly coupled to removed root hooks is not acceptable.

## Data Flow and Behavior Expectations

Behavior should remain the same for callers already using the new interface:

1. a typed binding lives on a `LockDefinition`
2. an execution surface attaches to that definition
3. `With(...)` binds input into an opaque request
4. registry and client validation continue to operate on the normalized use case graph

The cleanup changes authoring surfaces, not runtime semantics.

## Error Handling

No new runtime error categories are needed.

The breaking change is expressed by removed exported functions and updated docs/examples, not by introducing transitional runtime errors.

Any compile failures in tests, examples, or consuming fixtures should be resolved by migrating those callsites to the new API shape.

### 7. Benchmarks And Consumer Fixtures

This cleanup also applies to repo-owned compatibility surfaces outside the main package tree.

Required migration targets include:

- `benchmarks/...` files that still use removed APIs
- `external_consumer_surface_test.go` if it asserts removed symbols
- `release_workflow_surface_test.go` if it asserts workflow coverage tied to removed surfaces
- smoke fixtures under `testdata/externalconsumer/...`
- release-consumer smoke fixtures under `testdata/releaseconsumer/...`

The goal is that repository-owned consumer validation reflects the new public interface and does not depend on removed symbols.

## Verification Strategy

At minimum, verify:

1. removed APIs no longer exist in the root package source
2. repo code and docs no longer teach or depend on those APIs
3. benchmarks and repo-owned consumer fixtures no longer depend on removed APIs
4. touched tests pass
5. CI-parity commands from `AGENTS.md` pass before claiming completion
6. external-consumer smoke coverage is updated and verified at the repo-owned fixture level
7. the advanced strict authoring wrapper is removed and advanced composite compiles and behaves through the new definition-level strict model only

Recommended verification commands:

1. `go test ./...`
2. `GOWORK=off go test ./...`
3. `go test ./backend/redis/...`
4. `go test ./idempotency/redis/...`
5. `go test ./guard/postgres/...`
6. `go test -tags lockman_examples ./examples/... -run '^$'`
7. `GOWORK=off go test . -run 'TestCIWorkflowCoversExternalConsumerInstall|TestExternalConsumerSmokeFixtureImportsReleasedModules'`
8. `GOWORK=off go test . -run '^TestReleaseWorkflow'`

Also run targeted repository searches to confirm removed surface references are gone from canonical docs, examples, benchmarks, and repo-owned consumer fixtures.

## Acceptance Criteria

The change is complete when all of the following are true:

1. The root package no longer exports:
   - `DefineRun`
   - `DefineHold`
   - `DefineClaim`
   - `Strict`
   - `DefineCompositeMember`
   - `Composite`
2. The repository's canonical public docs teach only the definition-first interface.
3. Runnable examples use only the new interface.
4. Benchmarks and repo-owned consumer smoke fixtures use only the new interface.
5. Internal tests validate behavior through the new interface rather than through removed compatibility helpers.
6. No canonical doc or example frames the removed APIs as still available or merely deprecated.
7. Relevant test suites pass.

## Risks

### Compile breakage in broad test surface

The removed APIs are used in multiple tests, examples, and repo-owned validation fixtures. The mitigation is to migrate callsites mechanically and keep behavior-focused coverage intact.

### Documentation drift

Some docs may mention deprecation indirectly instead of naming the old APIs explicitly. The mitigation is to search broadly for both the symbol names and deprecation wording, then normalize docs to one story.

### Hidden legacy branching

Some internal code may still carry support logic for old authoring paths after the exported functions are removed. The mitigation is to inspect and delete legacy-only branches where safe, while preserving the new interface behavior.

### Composite path confusion

The repository has both root composite primitives and an advanced composite package. The mitigation is to keep docs and examples consistent with the advanced-interface direction so implementers do not accidentally migrate canonical material to a root primitive path that is no longer the intended public story.

## Implementation Outline

1. Add or update tests first for any legacy-only branch removal that needs behavior protection under the new API.
2. Remove deprecated exports from production code.
3. Rewrite affected tests.
4. Rewrite examples.
5. Rewrite docs and changelog language.
6. Run formatting and verification commands.

## User Outcome

After this change, the repository presents one coherent public story:

- one lock definition model
- one strictness model
- one composite model
- no deprecated compatibility surface in code or docs

That matches the user's request to drop deprecated parts entirely and update both code and documentation to the new interface.
