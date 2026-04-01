# DefineHold — Detached Lease Use Case Kind

**Date:** 2026-04-01  
**Status:** Approved

## Summary

Add `DefineHold[T]` as a third use case kind alongside `DefineRun` and `DefineClaim`. A Hold acquires a lease and returns a serializable token; a separate `Forfeit` call releases it later — potentially from a different process or goroutine. Designed for async job coordination (e.g. batch deletes, long-running workers) where the lock must outlive the acquiring call stack.

## Decisions

| Topic          | Decision                                                                                            |
| -------------- | --------------------------------------------------------------------------------------------------- |
| TTL renewal    | No renewal — Hold is TTL-bounded. `TTL(d)` sets the window; Forfeit is a best-effort early release. |
| Token format   | Self-contained, version-prefixed binary blob encoded as base64url. No external storage needed.      |
| Strict mode    | Not supported in v1. Explicitly noted for future addition.                                          |
| Engine pattern | New `lockkit/holds` package, parallel to `lockkit/runtime` (Run) and `lockkit/workers` (Claim).     |

## Public API

```go
// Definition
var ContractBatchDelete = lockman.DefineHold[ContractInput](
    "contract.batch_delete",
    lockman.BindResourceID("contract", func(in ContractInput) string { return in.EntityID }),
    lockman.TTL(15*time.Minute),
)

// Producer: acquire detached lease
req, err := ContractBatchDelete.With(ContractInput{EntityID: "abc"})
handle, err := client.Hold(ctx, req)
token := handle.Token() // opaque string → store in job metadata

// Consumer: release by token
req := ContractBatchDelete.ForfeitWith(token)
err = client.Forfeit(ctx, req)
```

`HoldHandle` exposes only `Token() string`. It carries no other state.

`ForfeitWith` packages `{useCaseCore, raw token string}` into an opaque `ForfeitRequest`. It does not decode the token — decoding happens inside `client.Forfeit`. This follows the same `UseCase.Method → Request → Client.Method` pattern as Run and Claim, avoiding Go's restriction on generic methods.

## Token Format

```
h1_<base64url(payload)>
```

Payload (binary, big-endian):

```
uint16  count of resource keys
  for each key:
    uint16  len(key)
    []byte  key bytes
uint16  len(ownerID)
[]byte  ownerID bytes
```

- Version prefix `h1_` allows future format changes without breaking existing tokens.
- Encodes `resourceKeys []string` (not just a single key) to match `backend.LeaseRecord.ResourceKeys`.
- Typical size: 30–80 bytes for standard inputs.
- `Forfeit` decodes the token and combines with `definitionID` derived from the use case definition.

## New Files

```
lockman/
  usecase_hold.go          HoldUseCase[T], DefineHold, .With() → HoldRequest, .ForfeitWith() → ForfeitRequest
  client_hold.go           client.Hold(), client.Forfeit()
  request.go               +HoldRequest (resourceKey, useCaseName, ownerID, useCaseCore), +ForfeitRequest (useCaseCore, token)

internal/sdk/
  token.go                 EncodeHoldToken / DecodeHoldToken

lockkit/holds/
  manager.go               holds.Manager — Acquire + Release, shutdown-aware
  manager_test.go

lockkit/definitions/
  types.go                 +DetachedAcquireRequest (DefinitionID, ResourceKeys, OwnerID), +DetachedReleaseRequest
```

Modified files:

```
lockman/registry.go        +useCaseKindHold constant
lockman/client.go          +holds *holds.Manager field
lockman/client_validation.go  +hasHoldUseCases, +Hold branch in buildClientPlan/translateUseCaseDefinition
lockman/errors.go          +ErrHoldTokenInvalid, +ErrHoldExpired
internal/sdk/usecase.go    +UseCaseKindHold constant, +nameDelimiter case 'h'
```

## Data Flow

### Hold (acquire)

1. `usecase_hold.go` — `.With(input)` validates binding, produces `HoldRequest{resourceKey, ownerID, useCaseCore}`
2. `client_hold.go` — `validateHoldRequest`: checks registry link, resolves identity
3. `client_hold.go` — calls `holds.Manager.Acquire(ctx, DetachedAcquireRequest{DefinitionID, ResourceKeys, OwnerID})`
4. `holds.Manager` — looks up `LockDefinition` from engine registry (for `LeaseTTL`), calls `backend.Acquire`, returns `backend.LeaseRecord`
5. `client_hold.go` — encodes token from `(resourceKeys, ownerID)`, returns `HoldHandle`

### Forfeit (release)

1. `usecase_hold.go` — `.ForfeitWith(token)` packages `{useCaseCore, raw token}` into `ForfeitRequest`
2. `client_hold.go` — `validateForfeitRequest`: checks registry link, decodes token → `(resourceKeys, ownerID)`, normalizes use case definition → `definitionID`
3. `client_hold.go` — calls `holds.Manager.Release(ctx, DetachedReleaseRequest{DefinitionID, ResourceKeys, OwnerID})`
4. `holds.Manager` — constructs minimal `backend.LeaseRecord{...}`, calls `backend.Release`

## Engine Layer — `lockkit/holds.Manager`

```go
type Manager struct {
    registry     registry.Reader
    driver       backend.Driver
    shuttingDown atomic.Bool
}

func NewManager(reg registry.Reader, driver backend.Driver) (*Manager, error)
// DetachedAcquireRequest carries DefinitionID, ResourceKeys, OwnerID only.
// LeaseTTL is resolved from the LockDefinition in the registry.
// No Ownership field — Hold doesn't need message delivery metadata.
func (m *Manager) Acquire(ctx context.Context, req definitions.DetachedAcquireRequest) (backend.LeaseRecord, error)
func (m *Manager) Release(ctx context.Context, req definitions.DetachedReleaseRequest) error
func (m *Manager) Shutdown()
```

`NewManager` validates the registry and asserts `driver != nil`. No strict or lineage driver checks in v1.

`Shutdown` sets the atomic flag; `Acquire` rejects new requests. No in-flight drain needed — Acquire is an instant backend call with no callback to wait on.

## Client Plan Changes

`clientPlan` gains `hasHoldUseCases bool`. `buildClientPlan` sets it when any registered use case has `kind == useCaseKindHold`. `client.New` constructs `holds.Manager` when `plan.hasHoldUseCases`.

Hold definitions register in the engine registry as `ExecutionKind = ExecutionSync`, `LockDefinition` — identical structure to Run definitions. The Hold/Run distinction exists only in the SDK layer.

`normalizeUseCase` maps `useCaseKindHold` to `sdk.UseCaseKindHold`. `toExecutionKind` maps it to `definitions.ExecutionSync`.

## Error Handling

New sentinel errors:

```go
ErrHoldTokenInvalid = errors.New("lockman: hold token is malformed or unrecognized")
ErrHoldExpired      = errors.New("lockman: hold lease has expired")
```

| Backend error                   | Client error                   |
| ------------------------------- | ------------------------------ |
| Malformed token                 | `ErrHoldTokenInvalid`          |
| `backend.ErrLeaseNotFound`      | `ErrHoldExpired` (TTL elapsed) |
| `backend.ErrLeaseOwnerMismatch` | `ErrBusy` (existing sentinel)  |
| `backend.ErrLeaseAlreadyHeld`   | `ErrBusy`                      |

`ErrHoldExpired` is distinct from `ErrBusy` so callers can treat an already-expired TTL as a non-fatal condition if appropriate.

## Testing Strategy

| Layer                      | Coverage                                                                                                                                                      |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `usecase_hold.go`          | `.With()` binding validation (empty key, nil binding, empty owner override)                                                                                   |
| `internal/sdk/token.go`    | Encode/decode round-trip; unknown version prefix rejected; N-key payloads                                                                                     |
| `lockkit/holds/manager.go` | Acquire returns lease; Release calls backend; Acquire after Shutdown rejected; unknown definition rejected                                                    |
| `client_hold.go`           | Happy path returns handle with correct token; Forfeit with expired token returns `ErrHoldExpired`; registry mismatch rejected; unregistered use case rejected |

## Out of Scope (v1)

- Strict mode (`Strict()` option) — noted for future support; token format's version prefix accommodates it
- Lineage support
- Composite holds
- Hold renewal / keepalive
