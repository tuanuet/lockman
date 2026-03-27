# Parent Over Composite Example

This example shows a case where one higher aggregate parent lock is enough even though the handler touches multiple sub-resources.

## What It Teaches

- several nested sub-resources inside the same aggregate do not automatically justify a composite
- a single parent boundary can be the right answer when the invariant is aggregate-wide
- composite is overkill when it does not add real coordination value

## Scenario

The flow touches two packages inside one shipment:

- `package-1`
- `package-2`

But the business invariant still belongs to the shipment aggregate, so the example protects `shipment:sh-123` with one parent lock instead of inventing a composite.

## Run

```bash
go run ./examples/phase2-parent-over-composite
```

## Output To Notice

- `aggregate lock: shipment:sh-123`
- `sub-resources involved: package-1,package-2`
- `teaching point: parent lock is enough, composite is overkill`

## Related Guide

See [`docs/lock-scenarios-and-best-practices.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-scenarios-and-best-practices.md) for the scenario on when a higher aggregate parent lock is enough.
