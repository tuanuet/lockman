# Lock Scenarios And Best Practices

## Who This Guide Is For

This guide exists for teams that already understand the lockman SDK surface but still need help choosing the right lock shape for a real product flow. It is scenario-driven and governance-driven: it focuses on how to make good design choices and how to review those choices consistently. It is not an API reference.

For application engineers, this guide explains how to map one concrete business flow to the right pattern, package, and key shape without guessing. The goal is to make day-to-day implementation decisions easier.

For architects and tech leads, this guide explains the review heuristics and governance trade-offs behind those choices so registry design can stay consistent across teams.

## Decision Framing

Teams often mix up several very different coordination problems and then choose the wrong lock shape.

ordinary lock contention means two callers want the same exact resource key at the same time. This is the normal “resource already held” case and usually points to retry, wait, or fail-fast decisions.

parent-child overlap rejection means two callers are touching different keys in the same resource tree, but the registry says they must not overlap. This is not the same as exact-key contention. In Phase 2a, it is an explicit runtime rule for validated parent-child definitions.

same-process reentrant rejection means one process tries to re-enter a protected section it already holds. That is a local admission-control problem, not a distributed lineage problem.

duplicate-delivery and idempotency concerns come from async delivery semantics. A queue consumer may receive the same logical message more than once even if lock acquisition itself is correct.

advisory presence is a visibility tool, not a correctness boundary. It can help operators or UIs understand whether a resource appears busy, but it is not a safe substitute for acquisition and policy enforcement.

## Quick Decision Guide

- If a direct caller is waiting for the result now, start with `runtime`. For the full execution-package comparison, see [`docs/runtime-vs-workers.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/runtime-vs-workers.md).
- If the flow starts from a replayable queue or background delivery, start with `workers`.
- If one aggregate-level invariant dominates the whole flow, prefer one parent lock.
- If sub-resources can be processed independently but still belong to an aggregate boundary, consider child locks with validated lineage.
- If one operation must hold multiple independent resources together, prefer a composite lock over manual nested acquires.
- If you only need an operator or UI hint, consider presence-check-only usage.
- If correctness depends on presence alone, stop and redesign around real acquisition.

## Pattern Catalog

### Parent Lock

#### What It Is

A parent lock protects one aggregate or root resource with one key, such as `order:123` or `account:abc`.

#### Good Fit

Use it when the business invariant is aggregate-wide and the safest answer is “only one writer for this aggregate at a time”.

#### Bad Fit

Do not use it when independent sub-resources truly need parallel processing and aggregate-wide serialization would become artificial contention.

#### Trade-Offs

Parent locks are simple to reason about and easy to govern, but they can reduce concurrency if they are applied too broadly.

#### Architecture And Registry Notes

Parent locks should usually be the default starting point. If a team proposes child locks, the review burden should be higher because they are making a concurrency promise, not just naming a sub-resource.

### Child Lock

#### What It Is

A child lock protects one sub-resource inside a known parent aggregate, such as `order:123:item:line-1`, with lineage back to the parent definition.

#### Good Fit

Use it when sub-resources can be processed independently and the system still needs the parent boundary to reject invalid overlap.

#### Bad Fit

Do not use it just because a sub-resource exists in the data model. If the invariant is still aggregate-wide, a child lock adds complexity without buying safe concurrency.

#### Trade-Offs

Child locks can increase throughput for independent sub-resources, but they require stronger registry discipline and clearer migration awareness after Phase 2a.

#### Architecture And Registry Notes

Every child definition should justify why parent-only locking is not enough. The registry should treat child locks as an explicit concurrency contract that must be reviewed carefully.

### Composite Lock

#### What It Is

A composite lock is one approved plan that acquires multiple member definitions together in canonical order.

#### Good Fit

Use it when one logical operation must hold multiple independent resources at once, such as a transfer that touches two accounts or an async job that needs several members before it can run.

#### Bad Fit

Do not use it when one parent lock already expresses the real invariant. Do not use it as a generic substitute for careful domain modeling.

#### Trade-Offs

Composites make multi-resource coordination safer than manual nested acquires, but they increase registry surface area and should stay small and intentional.

#### Architecture And Registry Notes

Composite membership should describe a business operation, not just a convenience bundle. Large composites are usually a sign that the lock boundary is too broad or under-modeled.

### Presence-Check-Only Usage

#### What It Is

Presence-check-only usage means a definition intentionally allows `CheckPresence` as an operational or UI hint without treating it as the correctness gate for writes.

#### Good Fit

Use it for dashboards, admin tooling, or operator visibility where “appears held” is useful context but not a source of truth.

#### Bad Fit

Do not use it to decide whether a correctness-critical write may proceed. Presence is advisory only and does not replace acquisition or policy validation.

#### Trade-Offs

Presence can improve observability and user communication, but over-trusting it leads to false safety assumptions.

#### Architecture And Registry Notes

Definitions that allow presence checks should still be reviewed as lock definitions first. `CheckOnlyAllowed` is a visibility choice, not a weaker form of coordination.

## Real Scenarios

## Best Practices

## Anti-Patterns

## Decision Matrix

## Related Docs And Examples
