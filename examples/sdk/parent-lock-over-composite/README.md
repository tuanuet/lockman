# Parent Over Composite Example

This workspace mirror tracks the public SDK interface for a case where one higher aggregate parent lock is enough even though the handler touches multiple sub-resources.

## Backbone concept

Choose the lock boundary that matches the real aggregate before reaching for composite coordination.

## What this example defines

- one named parent lock definition for the shipment aggregate
- one execution surface that protects `shipment:sh-123`
- one parent aggregate boundary even though multiple sub-resources are touched

## Why this shape matters

The flow touches two packages inside one shipment:

- `package-1`
- `package-2`

But the business invariant still belongs to the shipment aggregate, so the example protects `shipment:sh-123` with one parent lock instead of inventing a composite.

That keeps the named lock definition aligned with the actual business boundary. The example stays on the recommended definition-first SDK path even though the scenario itself is about choosing parent locking over composite locking.

## How to run

```bash
go run -tags lockman_examples ./examples/sdk/parent-lock-over-composite
```

## Output To Notice

- `aggregate lock: shipment:sh-123`
- `sub-resources involved: package-1,package-2`
- `teaching point: parent lock is enough, composite is overkill`

## Related Guide

See [`docs/lock-scenarios-and-best-practices.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-scenarios-and-best-practices.md) for the scenario on when a higher aggregate parent lock is enough.
