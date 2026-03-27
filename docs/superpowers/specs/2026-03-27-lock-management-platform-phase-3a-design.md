# Lock Management Platform Phase 3a Design

## Goal

Phase 3a adds strict execution plumbing and fencing-token issuance without pulling guarded persistence helpers into the same increment.

This phase exists to make strict mode real in runtime behavior rather than only in registry validation. After Phase 2 and Phase 2a, the codebase already exposes:

- `ModeStrict` in public definitions
- strict-mode registry validation
- standard-mode runtime and worker execution
- lineage-aware parent-child enforcement for standard mode

What is still missing is the execution plumbing needed before the full strict contract can be completed:

- managers must execute strict definitions intentionally
- drivers must issue monotonic fencing tokens
- lease and claim contexts must carry those tokens
- renewal and release must preserve strict ownership semantics

Phase 3a stops there. It does not yet add guarded-write repository contracts or persistence adapters. Those remain Phase 3b work, so this phase must not market strict mode as the finished strong-coordination story.

## Scope

### In Scope

- preview-quality strict execution for single-resource runtime paths
- preview-quality strict execution for single-resource worker paths
- fencing-token issuance from drivers for strict acquisitions
- fencing-token propagation through `LeaseContext` and `ClaimContext`
- renewal semantics for strict leases and claims
- driver contract expansion needed to support fencing
- strict-mode examples and docs

### Out Of Scope

- guarded-write repository helpers
- persistence adapters or SQL helpers
- composite strict execution
- strict composite fencing semantics
- strict-mode parent-child lineage enforcement
- strict child definitions that rely on `ParentRef` lineage behavior
- audit and tracing expansion beyond the current recorder model
- changing worker ack policy beyond what existing Phase 2 behavior already does

## Why This Phase Boundary

Three boundary choices were available:

### Option A: Token Plumbing Only

Add fencing tokens to types and driver contracts, but keep managers effectively standard-mode.

This is too weak. It creates future-facing fields without proving the runtime semantics that will depend on them.

### Option B: Strict Execution Plus Token Issuance

Add strict execution behavior and fencing tokens to the runtime and worker managers, but stop before repository guard helpers.

This is the recommended boundary. It makes strict mode executable and testable while still keeping persistence integration isolated for Phase 3b.

### Option C: Strict Execution Plus Guarded Writes

Implement strict mode together with guarded persistence abstractions and repository helpers.

This is too broad for one phase. It couples lock execution, token issuance, write-outcome normalization, and storage integration into one rollout.

## Design Summary

Phase 3a treats strict mode as preparatory execution support, not yet a complete application safety story.

The SDK may now:

- acquire a strict lease or claim
- obtain a fencing token from the driver
- propagate that token to the application callback
- renew and release the strict lease safely

The SDK may not yet:

- enforce guarded writes itself
- decide whether a database write was stale or accepted
- provide repository adapters that consume the fencing token

This keeps the phase boundary honest: strict execution becomes available for token propagation and integration testing, but documentation must still say that full persistence safety is not complete until Phase 3b guarded-write integration is adopted.

## Strict Mode Semantics In Phase 3a

### Execution Rules

For a single-resource definition with `Mode=strict`:

- runtime execution is allowed only through `ExecuteExclusive`
- worker execution is allowed only through `ExecuteClaimed`
- acquisition must go through a fencing-capable driver path
- a successful acquire yields a positive fencing token
- renewal preserves the same fencing token for the lifetime of that lease
- release uses the strict lease identity produced by acquire

### Failure Rules

- strict definitions remain fail-closed
- if the configured driver does not support fencing, manager construction fails
- if a strict acquire cannot obtain a fencing token, acquisition fails and the underlying driver error should propagate rather than being rewritten as a policy error
- if renewal loses the strict lease, the callback is cancelled and the execution ends with `ErrLeaseLost`

### What Phase 3a Does Not Promise

Phase 3a does not promise that strict mode alone prevents stale persistence writes. The token is issued and surfaced, but application storage layers still need guarded-write behavior that will arrive in Phase 3b. Until then, strict execution should be documented as preview or preparatory behavior rather than the completed strict contract from the platform design.

## Public API Changes

### Lease And Claim Context

`LeaseContext` and `ClaimContext` already contain `FencingToken`. Phase 3a makes that field meaningful.

Rules:

- standard-mode executions continue to expose `FencingToken=0`
- strict-mode executions must expose `FencingToken>0`

This preserves backward compatibility for existing standard-mode code.

### Error Surface

Phase 3a does not need to activate `ErrStaleToken` yet.

Reason:

- runtime and workers in this phase still do not perform guarded writes
- the SDK itself would have no first-class place to emit `ErrStaleToken`
- the error belongs more naturally to Phase 3b when guarded-write outcomes become public SDK behavior

### No New Public Guard Package Yet

`lockkit/guard` or repository helper packages do not ship in this phase. The type and package surface should stay focused on execution and fencing.

## Driver Contract Design

The current `drivers.Driver` contract is insufficient for strict mode because `Acquire` returns only a lease record and has no way to issue fencing metadata.

Phase 3a introduces an optional strict-driver capability alongside the existing driver contracts.

### Recommended Interface

```go
type FencedLeaseRecord struct {
    Lease        LeaseRecord
    FencingToken uint64
}

type StrictAcquireRequest struct {
    DefinitionID string
    ResourceKey  string
    OwnerID      string
    LeaseTTL     time.Duration
}

type StrictDriver interface {
    AcquireStrict(ctx context.Context, req StrictAcquireRequest) (FencedLeaseRecord, error)
    RenewStrict(ctx context.Context, lease LeaseRecord, fencingToken uint64) (FencedLeaseRecord, error)
    ReleaseStrict(ctx context.Context, lease LeaseRecord, fencingToken uint64) error
}
```

`RenewStrict` returns a refreshed `FencedLeaseRecord`:

- `Lease` must contain the updated lease timings after a successful renew
- `FencingToken` must remain identical to the token issued at acquire time

### Why A Separate Strict Capability

A separate capability is preferred over mutating the existing `Driver` methods because:

- standard-mode implementations stay compatible
- Phase 1 and Phase 2 tests remain largely stable
- managers can fail fast only when relevant strict definitions are present
- memory and Redis drivers can add strict support incrementally

### Token Semantics

For one `(definitionID, resourceKey)` strict boundary:

- each new successful acquire gets a strictly increasing token
- renew returns the same token
- release does not increment the token
- a later owner receives a higher token than any previous owner for that same boundary

Tokens are scoped to one strict lease namespace. They do not need to be globally increasing across all definitions.

## Manager Construction Rules

Phase 3a should follow the same detection pattern introduced for Phase 2a lineage support.

Recommended registry helpers:

```go
func RequiresStrictRuntimeDriver(reg Reader) bool
func RequiresStrictWorkerDriver(reg Reader) bool
```

Semantics:

- `RequiresStrictRuntimeDriver` returns true when any registered definition has `Mode=strict` and `ExecutionKind` of `sync` or `both`
- `RequiresStrictWorkerDriver` returns true when any registered definition has `Mode=strict` and `ExecutionKind` of `async` or `both`

This keeps manager construction aligned with the shared-registry pattern already used across runtime and workers.

### Runtime Manager

`runtime.NewManager(...)` must fail when:

- the registry contains any strict definition whose `ExecutionKind` is `sync` or `both`
- and the supplied driver does not implement `StrictDriver`

If the registry has no strict definitions, existing standard-only drivers remain valid.

### Worker Manager

`workers.NewManager(...)` must follow the same rule:

- strict definitions whose `ExecutionKind` is `async` or `both` require a strict-capable driver
- standard-only registries still work with existing drivers

This mirrors the lineage capability gate already used for Phase 2a.

## Runtime Execution Design

### Acquire Path

When `ExecuteExclusive` resolves a definition:

- if `def.Mode == standard`, keep current logic
- if `def.Mode == strict`, use strict-driver acquire

Strict execution still follows the current lifecycle:

1. resolve definition
2. build key
3. validate request
4. acquire strict lease
5. populate `LeaseContext.FencingToken`
6. execute callback
7. renew if needed
8. release strict lease

### Renewal Clarification

Phase 3a does not add a renewal loop to `runtime.ExecuteExclusive`.

That means:

- strict runtime execution must still complete within one lease TTL window in this phase
- `StrictDriver.RenewStrict` exists for contract completeness and for worker usage
- runtime strict renewals remain a later-phase concern unless the runtime execution model itself grows a renewal loop

This intentionally preserves the current asymmetry where worker execution has SDK-owned renewal and runtime execution does not.

### Reentrancy And Active Guards

Same-process reentrancy remains unchanged. The local guard key stays:

- definition ID
- resource key
- owner ID

Strict mode does not weaken reentrancy policy.

Phase 3a does not introduce a same-process reentrant execution fast-path. If the same runtime manager re-enters the exact same boundary, it is still rejected with the existing reentrancy error rather than executing a nested callback with copied lease state.

### Renewal

Strict renewals must preserve token identity:

- token stays constant for one acquired lease
- renewed `LeaseContext` deadline changes
- `FencingToken` does not change mid-execution

If renew fails because the lease is lost, the callback context is cancelled just as in standard mode.

### Internal State Note

The internal runtime lease holder should gain an optional fencing token alongside the existing lease metadata.

Recommended shape:

```go
type heldLease struct {
    lease         drivers.LeaseRecord
    lineage       *drivers.LineageLeaseMeta
    fencingToken  uint64
}
```

The zero value keeps standard mode unchanged. Non-zero means the held lease came from the strict driver path.

## Worker Execution Design

### Acquire Path

When `ExecuteClaimed` resolves a definition:

- standard mode keeps current behavior
- strict mode uses strict-driver acquire and stores the fencing token in `ClaimContext`

If a `strict + lineage` request somehow reaches `ExecuteClaimed` despite registry validation, the worker manager should still fail closed and return `ErrPolicyViolation` rather than attempting an undefined acquire path.

### Idempotency Requirement

Existing registry validation already requires idempotency for strict async and strict both definitions. Phase 3a keeps that rule and treats it as mandatory.

### Outcome Handling

Phase 3a does not change the existing worker terminal outcome mapping beyond carrying fencing metadata in the claim context. The worker runtime still:

- begins idempotency
- acquires the strict claim
- executes the callback
- persists terminal idempotency status
- releases the claim

What changes is that the worker callback now receives the fencing token needed by future guarded-write integration.

Strict-specific failure behavior in this phase is:

- manager construction failure because the driver lacks `StrictDriver` happens before worker execution begins, so it does not participate in worker outcome mapping
- strict acquire failure because a fencing token cannot be issued should propagate the underlying driver error
- those acquire-time driver errors continue to flow through the existing `OutcomeFromError(...)` default behavior rather than introducing any new strict-only outcome mapping in Phase 3a

### Internal State Note

The worker renewal state should mirror the runtime pattern by carrying an optional fencing token:

```go
type renewableLease struct {
    lease         drivers.LeaseRecord
    lineage       *drivers.LineageLeaseMeta
    fencingToken  uint64
}
```

Again, `0` preserves standard-mode behavior and non-zero marks a strict claim.

## Definitions And Registry Rules

Phase 3a keeps the current registry validation rules and clarifies them operationally:

- strict definitions require `FencingRequired=true`
- strict definitions require `BackendFailurePolicy=fail_closed`
- strict async and strict both definitions require idempotency
- strict composite members remain unsupported

No new definition fields are required in Phase 3a.

Additional Phase 3a restriction:

- strict definitions with `Kind=child` and a non-empty `ParentRef` remain unsupported
- standard child definitions may not reference strict parents because Phase 2a lineage validation already requires both sides of the lineage chain to remain `ModeStandard`

These restrictions should be enforced during registry validation rather than deferred until runtime or worker execution.

Reason:

- Phase 2a lineage enforcement is currently standard-mode focused
- strict lineage raises extra questions about ancestor overlap, token scope, and future guarded-write semantics
- Phase 3a should not partially invent strict lineage behavior without a dedicated design

Strict definitions with `Kind=parent` and no children referencing them remain fully supported as single-resource strict locks.

## Mode And Lineage Composition

Phase 3a combines two independent dimensions:

- lock mode: `standard` or `strict`
- lineage participation: no lineage or lineage-aware

The execution matrix is:

| Mode | Lineage | Driver Path | Phase 3a Status |
|---|---|---|---|
| `standard` | none | `Driver.Acquire` | supported |
| `standard` | present | `LineageDriver.AcquireWithLineage` | supported from Phase 2a |
| `strict` | none | `StrictDriver.AcquireStrict` | supported in Phase 3a |
| `strict` | present | blocked | unsupported in Phase 3a |

The blocked `strict + lineage` case is enforced by registry validation rather than by ad hoc runtime branching.

## Driver Implementations

### Redis Driver

Redis is the first production strict driver.

Recommended design:

- maintain one monotonic counter per strict lease namespace
- on successful strict acquire:
  - increment the counter
  - bind the resulting token to the acquired lease
- on renew:
  - verify the same owner still holds the lease
  - return the same token already associated with that lease
  - return a refreshed `LeaseRecord` inside the `FencedLeaseRecord`
- on release:
  - verify owner and token match the active lease

Recommended key families:

- lease record key
- fencing counter key
- token metadata key or encoded token field inside the lease payload

The exact Redis representation may be one Lua-backed atomic structure or a small atomic script set, but the driver must guarantee that lease acquisition and token issuance are one atomic decision.

Key namespace requirement:

- fencing counter keys must use a distinct prefix such as `lockman:lease:fence:{definition}:{resource}`
- any token metadata keys must also use a dedicated strict-mode namespace
- strict fencing keys must not collide with existing lease keys or lineage keys

### Memory Test Driver

The in-memory driver in `testkit` should also implement `StrictDriver`.

It must:

- issue deterministic increasing tokens per strict boundary
- preserve token across renew while returning a refreshed `FencedLeaseRecord`
- reject mismatched release attempts if token does not match the held lease

This is required so strict runtime and worker behavior can be tested without Redis in the main unit suite.

## Examples

Phase 3a should ship two examples:

### Strict Runtime Example

Teach:

- strict definition registration
- runtime callback receiving `FencingToken`
- repeated acquisitions for the same key returning larger tokens over time

### Strict Worker Example

Teach:

- strict async definition with idempotency
- worker callback receiving `FencingToken`
- strict claim lifecycle still using `workers`

Examples should not pretend to perform guarded persistence writes yet. They should explicitly say that the token is now available for the persistence layer, but guarded-write helpers come later.

## Documentation Changes

Phase 3a docs must update:

- `README.md`
- `docs/lock-definition-reference.md`
- runtime vs worker guidance where strict examples matter

Required messaging:

- strict execution plumbing is now available
- strict execution issues fencing tokens
- full persistence safety still requires Phase 3b guarded writes
- Phase 3a behavior is not the completed strict contract yet

## Observability Boundary

Phase 3a does not require a breaking change to `observe.Recorder`.

The current recorder contract remains valid:

- strict executions still call the existing acquire, contention, timeout, release, and active-lock hooks
- no new strict-only recorder methods are required in this phase

Operationally, strict executions are distinguished by definition metadata rather than by a new recorder API. Richer mode-aware telemetry remains a later observability increment.

## Testing Strategy

### Unit Tests

- runtime strict acquire returns non-zero fencing token
- worker strict claim returns non-zero fencing token
- renew preserves the same fencing token
- later reacquire yields a larger token than the previous lease
- manager construction fails if relevant strict definitions exist but driver lacks strict capability
- standard definitions still work with standard-only drivers

### Redis Integration Tests

- strict acquire issues increasing tokens
- renew preserves token
- release with wrong owner or wrong token is rejected
- strict runtime and strict worker both operate on Redis driver

### Regression Coverage

- existing standard-mode tests remain green
- existing lineage-aware tests remain green
- strict support does not change standard composite behavior

## Risks And Mitigations

### Risk: Strict Mode Looks Safer Than It Is

If docs overstate Phase 3a, adopters may assume stale persistence writes are already prevented.

Mitigation:

- document clearly that Phase 3a exposes fencing but not guarded writes
- keep Phase 3b called out in README and strict examples

### Risk: Driver Contract Churn

If fencing is bolted onto the existing driver methods carelessly, the standard driver API may become awkward.

Mitigation:

- add a separate strict-driver capability
- keep standard driver methods unchanged

### Risk: Composite Scope Creep

Strict composite execution is materially harder because the token scope is execution-level rather than member-level.

Mitigation:

- explicitly keep strict composite execution out of scope
- fail registry validation or manager execution if strict composite paths are attempted

## Deliverables

Phase 3a is complete when the codebase has:

- strict-capable driver contract
- strict support in runtime and workers for single-resource definitions
- fencing token issuance in Redis and memory drivers
- strict examples
- docs stating the exact phase boundary

Phase 3a is not complete until repository helpers, guarded write outcomes, and persistence adapters exist. Those belong to Phase 3b.
