# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [1.5.0] - 2026-04-08

- Add OTel and Prometheus observability examples:
  - New `examples/observability/otel` demonstrating OpenTelemetry sink integration
  - New `examples/observability/prometheus` demonstrating Prometheus metrics sink
- Add PrometheusSink for metrics observability:
  - New `observe/prometheus` package with Prometheus-compatible metrics export
  - Support counters for acquire, release, claim, hold operations
  - Support histograms for latency measurements
- Refactor OTel sink into dedicated submodule:
  - Move `OTelSink` to `observe/otel` submodule
  - Add span support for lock lifecycle events
  - Add metrics integration with outcome labels
- Remove deprecated datadog observability example:
  - Clean up unused datadog example code and dependencies
- Add LOCKMAN_REDIS_URL support to prometheus example:
  - Allow Redis backend configuration via environment variable
- Update Go version to 1.25.0 and synchronize dependencies across modules
- Unify LockKind type, add driver assertions, alias SDK errors:
  - Consistent LockKind across backend interfaces
  - Add driver interface assertions for compile-time verification
  - Alias root SDK errors in lockkit for compatibility

## [1.4.3] - 2026-04-05

- Add public `backend/memory` in-memory driver for unit testing:
  - New `backend/memory` package with `NewDriver()` constructor
  - Replaces `lockkit/testkit` as the public in-memory backend
  - Driver supports all core operations: acquire, release, extend, force release, and contention
- Refactor idempotency memory store into `idempotency/memory/` subpackage:
  - Move store implementation to `idempotency/memory/store.go`
  - Update all imports and tests to use new package path
- Move testkit utilities into `backend/memory` and remove `lockkit/testkit`:
  - Merge assertion helpers and memory driver into single `backend/memory` package
  - Update all example and test imports across root, advanced, benchmarks, and lockkit packages
- Update SKILL.md to reference new `backend/memory` package path

## [1.4.2] - 2026-04-04

- Add `FailIfHeldDef()` definition option for composite lock preconditions:
  - Mark a composite member as check-only — aborts with `ErrPreconditionFailed` if already held
  - Pre-check runs before any acquire begins; no members are acquired if a precondition fails
  - Check-only members excluded from callback `Lease.ResourceKeys`, guard tracking, and active-lock metrics
  - Can be combined with `StrictDef()` on the same definition
- Add `ErrPreconditionFailed` error sentinel to root SDK and lockkit errors
- Add composite authoring validation: panics on zero members or duplicate definitions
- Update `docs/advanced/composite.md`, `docs/errors.md`, and `SKILL.md` with fail-if-held coverage

## [1.4.1] - 2026-04-04

- Add `RunMultiple` and `HoldMultiple` client methods for acquiring multiple locks on the same definition in a single call:
  - `RunMultiple(ctx, []RunRequest, fn)` executes a function after acquiring all specified keys
  - `HoldMultiple(ctx, []HoldRequest)` acquires multiple locks and returns a releaser handle
  - New `ExecuteMultipleExclusive` engine for multi-key same-definition acquire
- Add examples for multiple lock usage:
  - `examples/sdk/multiple-run`
  - `examples/sdk/multiple-hold`
- Add `docs/multiple-lock.md` with full documentation for multiple lock patterns
- Update README and SKILL.md with multiple lock coverage

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
