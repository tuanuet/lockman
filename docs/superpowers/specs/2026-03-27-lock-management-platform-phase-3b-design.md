# Lock Management Platform Phase 3b Design

## Goal

Phase 3b completes the first real persistence-safety story for strict mode.

After Phase 3a, the codebase already exposes:

- strict execution for single-resource runtime paths
- strict execution for single-resource worker paths
- fencing-token issuance from strict-capable drivers
- fencing-token propagation through `LeaseContext` and `ClaimContext`

What is still missing is the persistence boundary that makes strict mode useful beyond lease ownership:

- repository-facing guarded-write contracts
- normalized guarded-write outcomes
- one concrete persistence adapter that proves stale writes are rejected by the database, not only by lock ownership

Phase 3b should add those pieces without turning the SDK into a general transaction framework or a Postgres-only API.

## Scope

### In Scope

- a new public `lockkit/guard` package
- generic guard metadata shared by runtime and worker strict paths
- normalized guarded-write outcomes
- helper functions that convert `LeaseContext` and `ClaimContext` into guard metadata
- worker-first guarded-write guidance and examples
- one concrete Postgres adapter pattern for guarded single-row `UPDATE`
- tests proving stale database writes are rejected when fencing tokens are older than the stored token

### Out Of Scope

- replacing or redesigning the existing worker idempotency subsystem
- a generic transaction/session abstraction
- multi-statement guarded-write orchestration
- `UPSERT` or insert-heavy persistence patterns
- strict composite guarded-write semantics
- strict lineage-specific guarded-write semantics
- automatic ack/retry policy changes inside `workers`
- adapters for databases other than Postgres in this phase

## Why This Phase Boundary

Three boundary choices were available.

### Option A: Postgres-Only Repository Helpers

Ship a concrete Postgres-focused helper package as the primary public surface.

This would move fastest in one database environment, but it would make strict-mode persistence look adapter-specific rather than contract-driven. That is the wrong long-term shape for the SDK.

### Option B: Generic Guard Contracts Plus One Concrete Postgres Adapter

Add a small public guard package, keep repository methods domain-specific in application code, and prove the pattern with one Postgres adapter for guarded single-row updates.

This is the recommended boundary. It keeps the user-facing API understandable, keeps adapter replacement possible, and still proves the persistence contract against a real database.

### Option C: Generic Guard Contracts Plus Transaction Framework

Add the guard package together with a cross-adapter repository/session framework for multi-statement operations.

This is too broad for one phase. It mixes guarded-write semantics, repository ergonomics, and transaction orchestration into one rollout before the single-row guarded-update story is proven.

## Design Summary

Phase 3b should make one statement true in real application storage:

> A stale strict worker may still run code, but it must not be able to commit a stale business write if its fencing token is older than the latest accepted token for that resource boundary.

The SDK should do this by standardizing:

- what guarded-write metadata looks like
- how handlers obtain that metadata from runtime or worker contexts
- how repositories report expected guarded-write results

The SDK should not do this by taking over application repositories or forcing one generic command framework on all business code.

The golden path should look like:

1. a strict worker callback receives `definitions.ClaimContext`
2. the handler converts that into `guard.Context`
3. the handler calls a domain-specific repository method
4. the repository performs a guarded `UPDATE`
5. the repository returns a normalized `guard.Outcome`

This keeps the strict-mode persistence story user-focused:

- generic where contracts should be shared
- domain-specific where business code should stay readable

## Public API Design

### New Package

Phase 3b should add:

```text
lockkit/guard/
```

This package owns only the shared strict persistence vocabulary. It should not own business repositories, SQL query builders, or worker policy decisions.

### Guard Context

Recommended public shape:

```go
package guard

type Context struct {
	LockID         string
	ResourceKey    string
	FencingToken   uint64
	OwnerID        string
	MessageID      string
	IdempotencyKey string
}
```

Purpose:

- `LockID` and `ResourceKey` identify the guarded boundary
- `FencingToken` is the ordering signal that the repository must respect
- `OwnerID` is retained for audit/debugging and optional persistence metadata
- `MessageID` and `IdempotencyKey` preserve worker metadata when available

Runtime paths will often populate only lock identity, resource key, owner identity, and fencing token. Worker paths may also populate message and idempotency metadata.

### Guard Outcome

Recommended public shape:

```go
type Outcome string

const (
	OutcomeApplied           Outcome = "applied"
	OutcomeDuplicateIgnored  Outcome = "duplicate_ignored"
	OutcomeStaleRejected     Outcome = "stale_rejected"
	OutcomeVersionConflict   Outcome = "version_conflict"
	OutcomeInvariantRejected Outcome = "invariant_rejected"
)
```

Meaning:

- `OutcomeApplied`: the guarded write committed successfully
- `OutcomeDuplicateIgnored`: the write was safely absorbed as a duplicate
- `OutcomeStaleRejected`: the fencing token was not fresh enough to apply the write
- `OutcomeVersionConflict`: an application-level optimistic concurrency rule failed
- `OutcomeInvariantRejected`: a business precondition failed without being an infrastructure error

The key rule is:

- `Outcome` is for expected business-classified write results
- `error` is for infrastructure failure or unexpected adapter failure

### Guard Conversion Helpers

Recommended helpers:

```go
func ContextFromLease(lease definitions.LeaseContext) Context
func ContextFromClaim(claim definitions.ClaimContext) Context
```

These helpers should:

- preserve `DefinitionID` as `LockID`
- preserve `ResourceKey`
- preserve `FencingToken`
- preserve `OwnerID`
- preserve `MessageID` and `IdempotencyKey` when available

These helpers should not attempt policy decisions. They exist to keep strict repository calls simple and consistent.

### No Generic Repository Interface

Phase 3b should not make a generic SDK repository interface like:

```go
Apply(ctx, guard, command) (Outcome, error)
```

the primary user-facing pattern.

That shape is too abstract for the main application experience. It makes repository calls harder to read and pushes domain logic into generic command payloads too early.

Instead, application repositories should stay domain-specific:

```go
outcome, err := orderRepo.ApplyOrderUpdate(ctx, guardCtx, cmd)
outcome, err := inventoryRepo.Reserve(ctx, guardCtx, cmd)
```

The SDK standardizes the shared metadata and outcomes, not the whole repository layer.

## Postgres Adapter Pattern

### Why Postgres First

Postgres is the first concrete adapter because this phase needs one real persistence proof, not another abstract contract without an implementation.

The adapter should remain a proof of the generic guard contract rather than a database-shaped public API. User code should depend on `lockkit/guard` types, not on Postgres-specific handler signatures.

### Recommended Schema Convention

The Postgres adapter should document a recommended business-row convention:

- `last_fencing_token bigint not null default 0`
- `updated_at timestamptz not null`
- `updated_by_owner text not null`

This is an adapter convention, not a mandatory SDK-wide schema contract.

Phase 3b should not require every future adapter to use the exact same columns. It should only require equivalent guarded-write semantics.

### Golden Path Operation

The first concrete Postgres pattern should be:

- guarded `UPDATE`
- against one existing business row
- using `last_fencing_token` on that row

Recommended query shape:

```sql
UPDATE orders
SET
  status = $1,
  last_fencing_token = $2,
  updated_at = NOW(),
  updated_by_owner = $3
WHERE id = $4
  AND last_fencing_token < $2
```

Semantics:

- if one row updates, return `OutcomeApplied`
- if zero rows update because the current token is greater than or equal to the incoming token, return `OutcomeStaleRejected`

This gives one atomic write rule that developers can understand immediately:

- newer token wins
- older or equal token loses

### Why Not A Side Guard Table First

A side guard table is a useful fallback for systems that cannot modify business-table schemas, but it should not be the default Phase 3b story.

Reasons:

- it makes transaction requirements appear immediately
- it splits one guarded boundary across multiple tables
- it is harder for developers to reason about on first adoption

Phase 3b should prove the simplest honest path first: business row plus fencing column.

### Why Not Upsert First

`UPSERT` and insert-heavy patterns are useful, but they add ambiguity that Phase 3b does not need yet:

- how should missing-row creation interact with stale tokens?
- what does duplicate creation mean for guard outcomes?
- how should app-level uniqueness and strict fencing interact?

Those questions are real, but they should come after the single-row guarded-update contract is established.

## Worker-First Execution Model

### Why Workers Are The Golden Path

Strict persistence safety matters most in async flows because those flows already have:

- duplicate delivery
- retries
- replay
- delayed or resumed handlers

That is where stale writes are most dangerous and least obvious.

This does not mean runtime should have a different public guard contract. It means worker usage should drive the first concrete example, tests, and documentation.

### Worker Handler Shape

The recommended worker path should look like:

```go
err := workersMgr.ExecuteClaimed(ctx, req, func(ctx context.Context, claim definitions.ClaimContext) error {
	guardCtx := guard.ContextFromClaim(claim)

	outcome, err := orderRepo.ApplyOrderUpdate(ctx, guardCtx, cmd)
	if err != nil {
		return err
	}

	switch outcome {
	case guard.OutcomeApplied:
		return nil
	case guard.OutcomeStaleRejected:
		return nil
	case guard.OutcomeDuplicateIgnored:
		return nil
	default:
		return someBusinessError(outcome)
	}
})
```

Important points:

- the repository method remains domain-specific
- `guard.ContextFromClaim(...)` keeps the repository call small and explicit
- stale or duplicate outcomes are business results, not infrastructure failures

### Relationship To Existing Idempotency

Phase 3b should not replace worker idempotency.

The current worker stack already has:

- idempotency validation in request handling
- idempotency store coordination before acquire
- terminal idempotency persistence after handler completion

That subsystem should remain intact. Phase 3b adds persistence guarding for stale writes; it does not create a second deduplication framework.

The relationship should be:

- Phase 2 idempotency protects duplicate message processing
- Phase 3b guarded writes protect stale database updates

The public `guard.Outcome` type should still include `OutcomeDuplicateIgnored` because worker flows already need that vocabulary, but the first Postgres guarded-update adapter mainly proves `OutcomeApplied` and `OutcomeStaleRejected`.

## Runtime Compatibility

Phase 3b is worker-first, but it should not become worker-only.

`guard.ContextFromLease(...)` should exist in the same phase so strict runtime handlers can use the same repository contract later:

```go
guardCtx := guard.ContextFromLease(lease)
outcome, err := repo.ApplyOrderUpdate(ctx, guardCtx, cmd)
```

This keeps the public vocabulary unified while still allowing docs and examples to emphasize the worker path first.

## Outcome And Error Rules

### Outcome Rules

Adapters should return `guard.Outcome` when the write reached the business-rule decision point.

Examples:

- token too old -> `OutcomeStaleRejected`
- duplicate work already absorbed -> `OutcomeDuplicateIgnored`
- app-specific invariant prevented update -> `OutcomeInvariantRejected`

### Error Rules

Adapters should return `error` when the write could not be classified because the persistence layer itself failed.

Examples:

- connection failure
- timeout
- SQL syntax/configuration failure
- transaction setup failure
- adapter misuse that prevents guarded classification

The SDK should not force callers to parse raw database errors to discover stale-write behavior.

## Testing Strategy

Phase 3b should prove the public contract and the first concrete adapter separately.

### Guard Package Tests

Unit tests for `lockkit/guard` should verify:

- `ContextFromClaim(...)` copies lock id, resource key, fencing token, owner id, message id, and idempotency key
- `ContextFromLease(...)` copies the runtime equivalents
- zero-value optional metadata stays zero-value rather than being invented

### Postgres Adapter Tests

Integration-style tests for the first Postgres adapter should verify:

- a newer fencing token updates the row and returns `OutcomeApplied`
- an older fencing token does not update the row and returns `OutcomeStaleRejected`
- an equal fencing token also returns `OutcomeStaleRejected`
- database failure returns `error`, not a fabricated success or stale outcome

### Worker-Oriented End-To-End Tests

Phase 3b should add at least one worker-oriented path that proves:

- strict worker callbacks receive a non-zero fencing token
- handlers can convert `ClaimContext` to `guard.Context`
- guarded repository writes can reject stale writers after a newer writer has already committed
- existing idempotency behavior remains intact

### Docs And Example Tests

The first example should be intentionally small:

- one strict worker
- one business row
- one guarded `UPDATE`
- one stale-rejection demonstration

The example should teach the pattern, not simulate a whole application.

## Package And Documentation Shape

Phase 3b should add a focused package surface:

```text
lockkit/guard/
lockkit/guard/postgres/   (or equivalent adapter package name)
```

Documentation should make the boundary explicit:

- `phase3a` exposed fencing tokens
- `phase3b` shows how to use them at the persistence boundary
- Postgres is the first adapter, not the permanent center of the public API

## Open Questions Deferred Beyond Phase 3b

The following are valid future questions but should not block this phase:

- should side guard tables be first-class helpers for schema-constrained systems?
- how should guarded `UPSERT` be normalized?
- should multi-statement guarded transactions get a dedicated abstraction?
- should guarded-write outcomes map directly into configurable worker ack policy later?
- should strict lineage and strict composite persistence guards share this exact outcome model?

## Recommendation

Phase 3b should implement:

- `lockkit/guard` with `Context`, `Outcome`, `ContextFromLease`, and `ContextFromClaim`
- worker-first examples and tests
- one Postgres adapter pattern for guarded single-row `UPDATE`
- domain-specific application repository methods that accept `guard.Context`

Phase 3b should not implement:

- a generic repository framework
- a new idempotency subsystem
- a cross-adapter transaction abstraction
- broad multi-row or `UPSERT` persistence support

This is the smallest phase that makes strict mode materially safer for real data while keeping the API understandable for application developers and replaceable across future persistence adapters.
