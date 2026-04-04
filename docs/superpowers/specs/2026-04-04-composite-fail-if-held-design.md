# Design: Composite Lock with FailIfHeld Members

## Summary

Add a `FailIfHeldDef()` definition option that marks a lock as a check-only precondition when used in a composite. During composite execution, the runtime performs a presence check for these members instead of acquiring them, aborting the chain if any such lock is already held.

## API Surface

```go
// Normal lock definitions
accountDef := lockman.DefineLock("account", binding)
ledgerDef := lockman.DefineLock("ledger", binding)

// Lock with fail-if-held precondition (functional option, consistent with StrictDef())
parentDef := lockman.DefineLock("parent", binding, lockman.FailIfHeldDef())

// Composite assembles them — ordering = argument order
var transferFundsDef = composite.DefineLock("transfer", parentDef, accountDef, ledgerDef)

// Attach as use case (existing pattern)
var transferFunds = composite.AttachRun("transfer.run", transferFundsDef, lockman.TTL(5*time.Second))
```

Default behavior: acquire normally. `FailIfHeldDef()` is opt-in.

## Runtime Execution Flow

1. **Plan phase** — Build composite member plans exactly as today, then canonicalize with the existing `policy.CanonicalizeMembers` ordering.
2. **Pre-check phase** — Iterate canonical members and, for each `FailIfHeld` member, call `runtime.Manager.CheckPresence(...)` rather than the backend directly. This preserves the existing `CheckOnlyAllowed` guard, backend ping, recorder metrics, and bridge presence events.
3. **Acquire phase** — Iterate canonical members again and acquire only the non-`FailIfHeld` members. If any acquire fails, release all previously acquired leases in reverse order and return the mapped acquire error.
4. **Callback** — Run the user callback with a lease context derived only from the acquired members.
5. **Defer** — Release only the acquired members in reverse order. Check-only members are never placed in the acquired slice and are never released.

Why two phases: pre-checks run first so the composite fails before taking any lock, while still preserving the existing runtime ordering semantics.

## Error Handling

- Runtime layer: add `lockkit/errors.ErrPreconditionFailed`, returned when a `FailIfHeld` member reports `definitions.PresenceHeld`.
- SDK layer: add a public sentinel such as `lockman.ErrPreconditionFailed` in root `errors.go`, and map `lockkit/errors.ErrPreconditionFailed` to it in `mapEngineError` so `Client.Run` does not leak a runtime-only sentinel.
- Acquire failures continue to follow the existing path: runtime returns acquire-layer errors such as `lockkit/errors.ErrLockBusy`, and `Client.Run` maps them to `lockman.ErrBusy` as usual.
- Owner information from presence checks is included in the formatted error text or a typed error wrapper around `ErrPreconditionFailed`; the sentinel itself remains compatible with `errors.Is`.

## Edge Cases

- **Duplicate lock definition** — `DefineLock` rejects if same definition appears twice.
- **Empty composite** — panics at define time.
- **Mixed types** — type system prevents mixing incompatible types via `LockDefinition[T]`.
- **FailIfHeld + StrictDef** — allowed. They are orthogonal: strict controls fencing, fail-if-held controls check-vs-acquire.

## Callback Semantics

- `Lease.ResourceKeys` includes only resource keys that were actually acquired.
- `Lease.LeaseTTL` and `Lease.LeaseDeadline` are computed only from acquired members, matching the existing `buildCompositeLeaseContext` behavior.
- `FailIfHeld` members act as preconditions and are intentionally invisible in the callback lease payload.

## Reentrancy And Active-Lock Accounting

- `FailIfHeld` members do not install active guards in `m.active` and do not contribute to active-lock counters.
- Only acquired members participate in reentrancy protection, active-lock metrics, and reverse-order release.
- Presence checks are advisory preconditions, not held leases, so they should not affect held-lock bookkeeping.

## Observability

- Presence checks reuse the existing runtime presence path, so they continue to emit `Recorder.RecordPresenceCheck(...)` metrics and bridge `EventPresenceChecked` events.
- Acquire and release observability remains unchanged for normal members.
- No new recorder interface methods are required for this feature.

## Files Changed

- `definition.go` — add `FailIfHeldDef()` DefinitionOption, add `failIfHeld` to `definitionConfig`, and expose it through `DefinitionConfig` so `def.Config().FailIfHeld` can be propagated by composite helpers.
- `binding.go` — add `failIfHeld` to `CompositeMember[T]` struct and `compositeMemberConfig`. Update `Member` and `MemberWithStrict` to propagate the flag from `def.Config()`, same pattern as `Strict`.
- `advanced/composite/api.go` — add composite-level validation for empty input and duplicate member definitions, and propagate `failIfHeld` when building members.
- `lockkit/definitions/types.go` — add `FailIfHeld` field to `LockDefinition`. Set `CheckOnlyAllowed = true` when `FailIfHeld` is set (reuses existing presence.go guard).
- `lockkit/runtime/composite.go` — two-phase execution: pre-check loop through `Manager.CheckPresence(...)`, then acquire loop for the rest. Exclude check-only members from guard installation, active counters, the acquired slice, and reverse-order release.
- `lockkit/errors/errors.go` — add `ErrPreconditionFailed` sentinel.
- `errors.go` — add the public SDK sentinel `ErrPreconditionFailed`.
- `client_validation.go` — update composite member translation so runtime `definitions.LockDefinition` receives `FailIfHeld` and `CheckOnlyAllowed` from composite member config, and map the new runtime precondition error to the public SDK error.
- `advanced/composite/api_test.go` — tests for: check passes when not held, check aborts when held, error includes owner info, mixed members with some fail-if-held and some normal, duplicate detection, empty composite panic.
- `definition_test.go` — tests for `FailIfHeldDef()` behavior.
- `lockkit/runtime/composite_test.go` — integration-level tests for two-phase execution, callback lease contents, and no guard/accounting side effects for check-only members.
