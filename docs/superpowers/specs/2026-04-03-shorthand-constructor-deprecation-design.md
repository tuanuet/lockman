# Shorthand Constructor Deprecation Design

## Status

Approved for specification drafting

## Goal

Deprecate the shorthand root-SDK constructors:

1. `DefineRun`
2. `DefineHold`
3. `DefineClaim`

and establish `DefineLock` plus `DefineRunOn`, `DefineHoldOn`, and `DefineClaimOn` as the only recommended public authoring model.

This deprecation pass should update code comments, user-facing documentation, canonical examples guidance, and release messaging without changing runtime behavior.

## Problem Statement

The repository already moved to a definition-first model in `v1.3.0`:

1. `DefineLock` defines shared lock identity
2. `DefineRunOn`, `DefineHoldOn`, and `DefineClaimOn` attach execution surfaces to that definition
3. the client and engine already reason in terms of shared definitions and execution kinds

But the shorthand constructors still exist as first-class public root-SDK entry points:

1. `DefineRun`
2. `DefineHold`
3. `DefineClaim`

Technically, they are now only sugar. Each shorthand constructor creates an implicit private definition and forwards into the corresponding `...On` constructor.

That creates an API-shape mismatch:

1. the implementation model is definition-first
2. the desired documentation model is definition-first
3. but the public API surface still suggests two valid authoring models instead of one canonical model plus one legacy compatibility layer

If the project goal is API purity, the shorthand constructors should stop looking like long-term primary APIs.

## Decision

The shorthand constructors should be deprecated now and removed in the next major release.

That means this design intentionally does all of the following:

1. keeps shorthand constructors compiling and behaving exactly as they do today
2. marks them deprecated at the Go API level
3. updates root docs to describe them as migration-only compatibility surfaces
4. updates canonical examples guidance to treat definition-first authoring as the only normal public path
5. announces that the next major version will remove the shorthand constructors from the root SDK

This design intentionally does not remove shorthand constructors in the current release line.

## Why Deprecate Instead Of Remove Now

Immediate removal would be a breaking shift in both API and release messaging.

Current repository context shows:

1. `v1.3.0` changelog explicitly says shorthand constructors are preserved
2. shorthand constructors still appear in tests, examples, docs, and wrapper packages
3. root-SDK users can still write valid single-use-case code with shorthand today

Removing them immediately would introduce unnecessary churn and would make the `v1.3.0` release line look unstable. Deprecation provides the right pressure without changing runtime behavior mid-line.

## Scope

In scope:

1. add Go deprecation comments to `DefineRun`, `DefineHold`, and `DefineClaim`
2. update root SDK docs so definition-first authoring is the only recommended path
3. rewrite shorthand mentions in canonical docs from `convenience path` to `deprecated compatibility path`
4. add explicit migration guidance from shorthand constructors to `DefineLock + ...On`
5. add changelog/release-note guidance for deprecation timing and next-major removal intent
6. update canonical examples and example indexes where shorthand is still described too positively
7. document how advanced wrapper packages relate to this deprecation in root documentation

Out of scope:

1. removing shorthand constructors in this change
2. changing shorthand runtime behavior
3. adding runtime warnings, logs, or panics for shorthand usage
4. renaming directories or restructuring package layout
5. redesigning `advanced/strict` or `advanced/composite` in the same pass

## API Policy After This Change

After this change, the public root-SDK policy should be:

1. `DefineLock`, `DefineRunOn`, `DefineHoldOn`, and `DefineClaimOn` are the only recommended root-SDK authoring APIs
2. `DefineRun`, `DefineHold`, and `DefineClaim` are deprecated compatibility helpers
3. new docs, examples, and recommendations should not present shorthand constructors as normal starting points
4. next major release removes the shorthand constructors from the root SDK

## Non-Goals

This design does not try to erase shorthand from history.

It does not require:

1. rewriting every historical or low-level example immediately
2. forcing advanced wrappers to adopt new names in the same release
3. changing internal forwarding implementation that already works
4. teaching runtime migration automation

## Required Files

The implementation must update at least these files for acceptance:

1. `usecase_run.go`
2. `usecase_hold.go`
3. `usecase_claim.go`
4. `README.md`
5. `CHANGELOG.md`
6. `docs/quickstart-sync.md`
7. `docs/quickstart-async.md`
8. `docs/production-guide.md`
9. `docs/runtime-vs-workers.md`
10. `docs/lock-definition-reference.md`
11. `docs/registry-and-usecases.md`
12. `examples/README.md`
13. `examples/sdk/shared-lock-definition/README.md`
14. `examples/sdk/sync-approve-order/README.md`
15. `examples/sdk/async-process-order/README.md`
16. `examples/sdk/shared-aggregate-split-definitions/README.md`
17. `examples/sdk/parent-lock-over-composite/README.md`
18. `examples/sdk/sync-fenced-write/README.md`

These files are the bounded acceptance surface for this pass. Files outside this list may be reviewed, but they do not need to change unless the user explicitly expands scope later.

## Definitions For Review

For this spec, the following terms are normative:

1. `first substantial code example` means the first multi-line Go code block in `README.md` that shows SDK authoring or execution, not an install command, import-only block, or isolated type declaration
2. `canonical docs` means the files listed in `Required Files`
3. `deprecated compatibility path` means wording that explicitly says shorthand constructors are deprecated, remain fully functional in the current release line for compatibility, and should not be used for new code
4. `example positioning` means the combined effect of example README prose, example index prose, and linked example recommendations; it does not require changing example source code unless a source snippet is embedded in one of the required docs

## Deprecation Comment Rules

Each shorthand constructor must receive a standard Go deprecation comment directly above the exported function.

Required messages:

1. `DefineRun`
   - `Deprecated: use DefineLock plus DefineRunOn.`
2. `DefineHold`
   - `Deprecated: use DefineLock plus DefineHoldOn.`
3. `DefineClaim`
   - `Deprecated: use DefineLock plus DefineClaimOn.`

These comments should be short, mechanical, and unambiguous.

They should not:

1. contain migration essays
2. apologize for the change
3. suggest that shorthand remains equally preferred

## Runtime Behavior Requirements

This pass must not change runtime behavior.

The shorthand constructors must continue to:

1. accept the same parameters
2. create implicit private definitions the same way they do today
3. return the same public use case types
4. preserve existing test behavior and external call patterns

No runtime warnings should be emitted from constructors or client methods.

Example source files may remain shorthand-based in this pass unless a required documentation file embeds a source snippet from them and presents it as recommended new code.

## Documentation Positioning Rules

After this change, the docs should use the following positioning:

1. definition-first authoring is the default public SDK path
2. shorthand constructors are deprecated and exist only for migration compatibility
3. new code should not be authored with shorthand constructors
4. next major release will remove the shorthand constructors from the root SDK

This positioning must appear explicitly in:

1. `README.md`
2. `docs/lock-definition-reference.md`
3. `docs/quickstart-sync.md` or `docs/quickstart-async.md`
4. `CHANGELOG.md`
5. either `docs/production-guide.md` or `docs/registry-and-usecases.md`

## README Requirements

The root `README.md` already teaches definition-first authoring. This pass should now harden the stance.

The current `When Shorthand Is Enough` section should be revised so that it no longer presents shorthand as an endorsed convenience path.

It should instead say that:

1. shorthand constructors are deprecated
2. they remain fully functional in the current release line for compatibility
3. new code should use `DefineLock + ...On`
4. the next major release will remove the shorthand constructors

The first substantial code example in the README must remain definition-first.

For acceptance, the first substantial code example in `README.md` must include `DefineLock` and at least one of `DefineRunOn`, `DefineHoldOn`, or `DefineClaimOn`.

## Quickstart Requirements

The quickstarts should continue to teach runnable authoring, but they must stop sounding like shorthand is equally healthy.

Rules:

1. the first code sample in each quickstart remains definition-first
2. shorthand references, if present, must be labeled deprecated
3. each quickstart that mentions shorthand must direct the reader toward `DefineLock + ...On` for new code

Only `docs/quickstart-sync.md` and `docs/quickstart-async.md` are required in this pass.

## Migration Guidance Requirements

The docs should include explicit mechanical migration guidance.

At minimum, one root documentation page should show these transformations:

1. `DefineRun(...)` -> `DefineLock(...)` + `DefineRunOn(...)`
2. `DefineHold(...)` -> `DefineLock(...)` + `DefineHoldOn(...)`
3. `DefineClaim(...)` -> `DefineLock(...)` + `DefineClaimOn(...)`

This migration guidance should be concise and code-first.

It should make clear that:

1. the new explicit definition may remain private to one package
2. sharing is not required to justify the migration
3. the gain is one consistent API model, not only shared identity reuse

This migration guidance must live in either `docs/lock-definition-reference.md` or `README.md`. It may appear in both, but at least one is required.

## Example Positioning Rules

Canonical SDK examples should reinforce one message:

1. `examples/sdk/shared-lock-definition` is the starting point
2. shorthand examples are still valuable as migration or focused scenario examples
3. but they are not the canonical authoring model anymore

This pass is about example positioning, not mandatory source rewrites. Example source code may remain shorthand-based if the surrounding canonical docs explicitly frame that code as deprecated compatibility or focused legacy migration material.

For the shorthand-based example READMEs, acceptance requires explicit wording that the example demonstrates a deprecated shorthand constructor kept temporarily for migration compatibility.

This applies at minimum to:

1. `examples/sdk/sync-approve-order/README.md`
2. `examples/sdk/async-process-order/README.md`

For other canonical SDK example READMEs that still use shorthand internally, the README should either:

1. call out that the example is using shorthand for a focused scenario and that shorthand is deprecated, or
2. remain unchanged in source code while the README frames the example as deprecated-API coverage rather than recommended new code

This design does not require all canonical examples to be rewritten to explicit definitions immediately, but it does require their positioning to stop treating shorthand as healthy default code.

The canonical example inventory for this rule is bounded to the README files listed in `Required Files`. Other examples are out of scope unless linked directly by a required file in a way that would contradict the new deprecation policy.

## Changelog Requirements

`CHANGELOG.md` must record the deprecation explicitly.

Required content:

1. shorthand constructors are now deprecated
2. use `DefineLock`, `DefineRunOn`, `DefineHoldOn`, and `DefineClaimOn` for new code
3. shorthand constructors remain behavior-compatible for the current release line
4. shorthand constructors will be removed in the next major release

The changelog must not imply that shorthand behavior changed in the current line.

## Advanced Package Positioning

This pass should avoid broad redesign of advanced packages, but it still needs a policy statement.

Recommended policy:

1. do not change `advanced/strict.DefineRun` or `advanced/composite.DefineRun` in this pass
2. do not deprecate advanced wrappers yet unless there is a separate approved design
3. `README.md` or `docs/production-guide.md` must include this message in substance: advanced packages are specialized surfaces and are outside the scope of this root-SDK shorthand deprecation pass

This keeps the deprecation pass focused and avoids mixing root-SDK cleanup with advanced API redesign.

## Acceptance Criteria

This design succeeds when all of the following are true:

1. `DefineRun`, `DefineHold`, and `DefineClaim` each have the required Go deprecation comment
2. shorthand runtime behavior remains unchanged
3. `README.md` and `docs/lock-definition-reference.md` describe `DefineLock + ...On` as the recommended authoring model for new code
4. `README.md` and `docs/lock-definition-reference.md` describe shorthand constructors as deprecated but fully functional compatibility helpers in the current release line
5. at least one root doc includes explicit mechanical migration examples for all three shorthand constructors
6. `README.md` keeps a definition-first first substantial code example
7. `README.md` no longer describes shorthand as a recommended convenience path for new code
8. `examples/sdk/sync-approve-order/README.md` and `examples/sdk/async-process-order/README.md` explicitly state that shorthand is deprecated and retained for compatibility
9. `CHANGELOG.md` states that shorthand is deprecated now and removed in the next major release
10. no runtime logging, warning emission, or behavior changes are added for shorthand usage
11. the advanced-wrapper policy is stated in either `README.md` or `docs/production-guide.md`

## Verification

Required verification commands:

1. `go test ./...`
2. `GOWORK=off go test ./...`
3. `go test ./backend/redis/...`
4. `go test ./idempotency/redis/...`
5. `go test ./guard/postgres/...`
6. `go test -tags lockman_examples ./examples/... -run '^$'`

Required read-through checks:

1. verify the three shorthand functions carry the exact intended deprecation comments
2. verify the first substantial README code sample remains definition-first
3. verify `README.md`, `docs/lock-definition-reference.md`, and the selected quickstart use wording that means: deprecated, fully functional in the current release line, and not recommended for new code
4. verify `examples/sdk/sync-approve-order/README.md` and `examples/sdk/async-process-order/README.md` explicitly label shorthand deprecated and compatibility-only for current users
4. verify changelog wording promises future removal without implying current breakage
5. verify no required doc presents shorthand source snippets as recommended new code

## Implementation Notes

This should be a focused API-positioning change, not a broad refactor.

Implementation priorities:

1. mark deprecations in code first
2. align root docs and migration guidance second
3. align canonical example wording third
4. leave advanced wrapper redesign for a separate approved design if needed

The goal is to make the public SDK tell one coherent story:

1. definition-first is the model
2. shorthand is legacy compatibility
3. removal is coming at the next major boundary
