# Shared Aggregate Runtime/Worker Example

This example shows one aggregate boundary touched by both a direct human-action path and a background-worker path.

## What It Teaches

- The aggregate key can stay the same across sync and async lifecycles.
- The teaching case uses split definitions:
  - `OrderApprovalSync`
  - `OrderApprovalAsync`
- The point is boundary clarity, not aggressive deduplication of registry entries.

## Why Split Definitions Here

The example uses split sync and async definitions over the same aggregate key boundary because the execution lifecycles are different:

- the sync path is a direct human-triggered `runtime` flow
- the async path is a message-driven `workers` flow

That does not automatically mean `ExecutionKind=both` is wrong. A single shared definition can still be acceptable when both paths really protect the same business meaning and deserve the same review semantics. This example keeps them split because it is easier to teach and easier to govern.

One important nuance: split definitions over the same key shape do not automatically create one shared lease namespace. If you need `runtime` and `workers` to contend on the exact same lock record, use one shared `ExecutionKind=both` definition instead.

## Run

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/phase2-shared-aggregate-runtime-worker
```

## Output To Notice

- `runtime path: acquired order:123`
- `runtime definition: OrderApprovalSync`
- `worker path: claimed order:123`
- `worker definition: OrderApprovalAsync`
- `shared aggregate key: order:123`
- `teaching point: split sync and async definitions can still share one aggregate boundary`

## Related Guide

See [`docs/lock-scenarios-and-best-practices.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-scenarios-and-best-practices.md) for the scenario family on sync and async shared boundaries.
