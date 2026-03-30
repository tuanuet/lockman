# Parent-Child Overlap Example

This example is the clearest demonstration of runtime parent-child overlap enforcement.

## What It Shows

- `runtime.ExecuteExclusive` now enforces parent-child overlap directly
- The enforcement happens across different managers that share the same distributed-capable driver contract
- Parent-held-child and child-held-parent conflicts are both rejected
- `CheckPresence` is not involved; overlap enforcement comes from lineage-aware acquire paths

## Prerequisites

- No external services for this example run
- The example uses the in-memory lineage-aware driver, but the same runtime contract is what Redis-backed distributed enforcement follows

## Run

```bash
go run ./examples/core/parent-child-overlap
```

## Flow

1. Register `OrderParentLock` and `OrderItemLock` with `ParentRef`.
2. Build two runtime managers that share the same lineage-aware driver.
3. Hold the child lock in one manager and try to acquire the parent in the other.
4. Hold the parent lock in one manager and try to acquire the child in the other.
5. Observe both conflict directions return `ErrOverlapRejected`.

## Output To Notice

- `scenario child-held-parent-rejected: overlap rejected`
  This proves a live child lease blocks the parent acquire.
- `scenario parent-held-child-rejected: overlap rejected`
  This proves a live parent lease blocks the child acquire.
- `note: phase 2a runtime now enforces parent-child overlap across managers and goroutines`
  This is the migration takeaway: `ParentRef` is no longer metadata only.
- `shutdown: ok`
  This proves both managers drained cleanly after the scenarios.

## When To Use This Example

Use this when your team previously assumed parent-child overlap was only enforced through composite plans. This example shows the new direct single-lock enforcement path.
