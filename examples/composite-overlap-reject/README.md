# Composite Overlap Reject Example

This example shows reject-first overlap policy inside one composite request on the memory driver.

## What It Shows

- A parent lock and child lock defined in the same resource tree
- A composite definition that tries to acquire both at once
- Policy validation during execution that rejects the overlap before the callback runs

## Prerequisites

- No external services

## Run

```bash
go run ./examples/composite-overlap-reject
```

## Flow

1. Register `OrderParentLock`.
2. Register `OrderItemLock` as a child of that parent with `OverlapReject`.
3. Register a composite that includes both definitions.
4. Execute the composite against the same order tree.
5. Observe that the callback never runs because the runtime rejects the overlap first.

## Output To Notice

- `overlap outcome: rejected`
  This proves the composite path rejected the policy violation before callback execution.
- `shutdown: ok`
  This proves the runtime manager stayed healthy after the rejected attempt.

## When To Use This Example

Use this when you want to understand the difference between "ordinary lock contention" and "the request itself is invalid because its parent/child members overlap".
