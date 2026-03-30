# Errors

Public SDK errors are meant to tell you what to do next.

## Contention And Timing

- `lockman.ErrBusy`: another owner currently holds the resource
- `lockman.ErrTimeout`: acquire timed out
- `lockman.ErrOverlapRejected`: overlap or lineage rules rejected the request

## Async Delivery

- `lockman.ErrDuplicate`: the delivery was already completed or failed earlier
- `lockman.ErrIdempotencyRequired`: a claim use case needs an idempotency store

## Setup And Wiring

- `lockman.ErrRegistryRequired`: client startup is missing a registry
- `lockman.ErrUseCaseNotFound`: request was built from an unregistered use case
- `lockman.ErrRegistryMismatch`: request belongs to a different registry
- `lockman.ErrIdentityRequired`: the effective `OwnerID` is empty
- `lockman.ErrBackendRequired`: the client needs a backend
- `lockman.ErrBackendCapabilityRequired`: the configured backend cannot satisfy the registered use cases

## Runtime State

- `lockman.ErrShuttingDown`: the client is shutting down
- `lockman.ErrLeaseLost`: the lease was lost before the callback completed
- `lockman.ErrInvariantRejected`: a runtime invariant rejected the execution

If you are seeing lower-level internal engine errors in application code, you are below the default SDK surface. Public-interface workspace examples live under `examples/sdk/`, lower-level scenario examples live under `examples/core/`, and published adapter-backed runnable copies still live in the adapter modules.
