# Shared Definition Contention Example

Archived note: the runnable Go package was removed from the root module during adapter-module extraction. This README remains as historical guidance only and is not part of released-root verification.

This example shows the opposite choice from the split-definition teaching case: one shared `ExecutionKind=both` definition is used by both `runtime` and `workers`, so both paths contend on the exact same lock record.

## What It Teaches

- one shared definition can be used from both execution packages
- `runtime` and `workers` will both see `lock busy` when they race on the same shared definition and key
- this is the right teaching case when you want one real contention boundary, not just one shared business key shape

## Scenario

Assume the sync approval path and the async approval path are truly the same business boundary. In that case, the definition itself can be shared:

- definition: `OrderApprovalShared`
- execution kind: `both`
- key shape: `order:{order_id}`

Because the definition is shared, the lock backend stores one shared lease namespace for `order:123`.

## Status

This root path is archived. Keep using it only as historical documentation while the adapter-module refactor is in flight.

## Output To Notice

- `runtime path: acquired order:123`
- `worker path during runtime lock: lock busy`
- `worker path: claimed order:123`
- `runtime path during worker claim: lock busy`
- `shared definition: OrderApprovalShared`
- `teaching point: one ExecutionKind=both definition creates one shared contention boundary across runtime and workers`

## Related Guide

See [`docs/lock-scenarios-and-best-practices.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-scenarios-and-best-practices.md) for the sync-versus-split definition guidance.
