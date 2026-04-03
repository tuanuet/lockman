# Shared Aggregate Split-Definitions Example

This workspace mirror tracks the public SDK interface. Its `main.go` is gated behind the `lockman_examples` build tag so default root verification does not depend on sibling adapter modules.

This example shows one aggregate boundary touched by both a direct human-action path and a background-worker path.

## Backbone concept

One business aggregate can keep one boundary while still choosing separate execution surfaces and separate shorthand definitions for sync and async lifecycles.

## What this example defines

- one shorthand sync lock definition for `OrderApprovalSync`
- one shorthand async lock definition for `OrderApprovalAsync`
- one shared aggregate resource key boundary: `order:123`

The aggregate key stays the same across both flows even though the execution surfaces differ.

## Why this shape matters

The example uses split sync and async definitions over the same aggregate key boundary because the execution lifecycles are different:

- the sync path is a direct human-triggered `runtime` flow
- the async path is a message-driven `workers` flow

That does not automatically mean `ExecutionKind=both` is wrong. A single shared definition can still be acceptable when both paths really protect the same business meaning and deserve the same review semantics. This example keeps them split because it is easier to teach and easier to govern.

This SDK version focuses on one public-client flow that issues both `Run` and `Claim` requests against the same aggregate boundary.

## How to run

```bash
go run -tags lockman_examples ./examples/sdk/shared-aggregate-split-definitions
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
