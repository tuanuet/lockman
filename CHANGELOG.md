# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [1.4.0] - 2026-04-03

- **Breaking Change**: Remove deprecated root-SDK shorthand constructors:
  - `DefineRun`, `DefineHold`, `DefineClaim` no longer exist
  - Use `DefineLock` + `DefineRunOn`/`DefineHoldOn`/`DefineClaimOn` instead
- Remove deprecated `Strict()` UseCaseOption - use `StrictDef()` DefinitionOption instead
- Remove deprecated `DefineCompositeMember` and `Composite(...)` - use `advanced/composite.DefineLock` and `AttachRun` instead
- Remove `advanced/strict.DefineRunOn` wrapper - use root API with `StrictDef()` instead
- Align advanced composite with supported strict model - strict composite members are rejected
- Update all examples, docs, and tests to use definition-first authoring only

## [1.3.1] - 2026-04-03

- Split Redis contention benchmarks to distinguish same-owner and distinct-owner workloads:
  - Keep `BenchmarkSyncLockLockmanRunRedisContention` as the same-owner case
  - Add `BenchmarkSyncLockLockmanRunRedisContentionDistinctOwners` for independent-owner contention
- Clarify performance comparisons against `redislock` by separating local same-owner guard effects from true multi-owner Redis contention

## [1.3.0] - 2026-04-03

- Add definition-first shared lock authoring with `LockDefinition[T]` and explicit constructors:
  - `DefineLock`, `DefineRunOn`, `DefineHoldOn`, `DefineClaimOn`
  - Preserve shorthand constructors with implicit per-use-case definitions
- Normalize shared definitions in client planning and engine registration:
  - Deduplicate shared definition registration
  - Compute execution kind across attached use cases (`ExecutionSync`, `ExecutionAsync`, `ExecutionBoth`)
  - Keep public `DefinitionID()` name-facing while using internal shared definition IDs for execution
- Add shared-definition option validation and strictness behavior:
  - Treat strictness at definition level
  - Reject conflicting non-zero `TTL` and `WaitTimeout` across use cases sharing one definition
- Extend composite run use cases to support shared-definition members with projection-based member APIs while preserving legacy `Composite(...)` compatibility
- Add definition-level force release capability:
  - New optional backend contract `ForceReleaseDriver`
  - Redis implementation to idempotently cleanup lease and strict-state keys
- Add and move shared lock definition examples to the SDK examples layer:
  - `examples/sdk/shared-lock-definition`
  - README updates for shared-definition guidance and examples layout

## [1.1.0] - 2026-03-31

- Add Phase 3c observability and inspection support:
  - `observe` package: event model, bounded async dispatcher, OTel adapter
  - `inspect` package: in-memory store, admin HTTP handlers, SSE streaming
  - Additive `WithBridge()` options for runtime and worker managers
  - Root SDK `WithObservability()` wiring with process-local state
- Backward-compatible: existing callers compile unchanged

## [1.0.0] - 2026-03-30

- Release the stable `github.com/tuanuet/lockman` root SDK module.
- Publish the user-first `Run` and `Claim` SDK path with example-driven docs and adapter modules.
