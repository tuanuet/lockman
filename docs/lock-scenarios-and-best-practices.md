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

## Scenario Families

### Single Aggregate Ownership

### Approve One Order

#### Problem

One synchronous API or command approves a single order and must serialize aggregate-wide state transitions.

#### Recommended Pattern

Use one parent lock. For this scenario, one parent lock is the clearest boundary because the invariant is at the order level, not the item level.

#### Recommended Execution Package

Use `runtime`.

#### Why This Choice

The caller is waiting now, and one parent lock keeps the coordination model simple. This is the baseline case where one parent lock plus `runtime` is usually enough.

#### Example Key Shape

`order:{order_id}` -> `order:123`

#### Best Practices

Keep the definition business-readable, keep the key stable, and treat the approval as one aggregate transition instead of many smaller pseudo-locks.

#### Common Mistakes

Do not introduce child locks just because the order contains items. If approval semantics are aggregate-wide, extra granularity only adds noise.

#### Architecture Note

Registry review should ask whether the team is really protecting one aggregate invariant. If the answer is yes, parent lock should remain the default.

### Aggregate Versus Sub-Resource Concurrency

### Update One Order Item Under Order-Level Invariants

#### Problem

One flow updates a single item inside an order, but the order still has higher-level invariants such as totals, status, or fulfillment boundaries.

#### Recommended Pattern

Use a child lock with lineage when item-level parallelism is intentional. If the item is not truly independent, parent-only is simpler.

#### Recommended Execution Package

Use `runtime` for sync updates or `workers` for async updates, but keep the same parent-child lineage model.

#### Why This Choice

This is the classic trade-off between child with lineage and aggregate serialization. Choose child with lineage only when item operations are genuinely independent enough to benefit from concurrency. If that claim is weak, parent-only is simpler and safer. Phase 2a overlap behavior now matters here because parent and child overlap is rejected directly.

#### Example Key Shape

Parent: `order:{order_id}` -> `order:123`  
Child: `order:{order_id}:item:{item_id}` -> `order:123:item:line-1`

#### Best Practices

Document why item-level concurrency is intentional, not accidental. Review whether the update changes aggregate-wide fields before approving a child definition.

#### Common Mistakes

Do not say “the data model has items, so we need item locks”. That is not enough. Child locks are justified by concurrency intent, not by table shape.

#### Architecture Note

This scenario should trigger a higher review bar. The team is asserting a concurrency contract, not just naming a nested resource.

### Multi-Resource Coordination

### Transfer Between Two Accounts

#### Problem

One operation moves value between two independent accounts and must coordinate both resources safely.

#### Recommended Pattern

Use a composite lock with the two account members.

#### Recommended Execution Package

Use `runtime` for direct request/response transfers or `workers` for queued transfer requests.

#### Why This Choice

Composite execution is the safe fit because two manual acquires are inferior. Manual nested acquires spread ordering and rollback logic into application code, while a composite keeps canonical ordering and all-or-nothing semantics inside the SDK.

#### Example Key Shape

`account:{account_id}` -> `account:A`, `account:B`

#### Best Practices

Keep the composite small and name it after the business operation rather than the member list. Review whether both members are truly required.

#### Common Mistakes

Do not manually call acquire twice in app code and hope the call order stays consistent forever.

#### Architecture Note

Composite definitions should represent approved business workflows. They should not become a general-purpose escape hatch for arbitrary multi-lock code.

### One Higher Aggregate Parent Lock Is Enough, Composite Is Overkill

#### Problem

One business flow touches several sub-resources inside the same aggregate, and the team is tempted to model it as a composite only because more than one nested thing is involved.

#### Recommended Pattern

Use one higher aggregate parent lock.

#### Recommended Execution Package

Use `runtime` for the direct synchronous teaching case.

#### Why This Choice

Composite is overkill when one higher aggregate parent lock already captures the real invariant. Multiple sub-resources inside the same aggregate do not automatically justify multi-resource coordination.

#### Example Key Shape

`shipment:{shipment_id}` -> `shipment:sh-123`  
Example: [`examples/phase2-parent-over-composite/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-parent-over-composite/README.md)

#### Best Practices

Ask whether the real correctness rule is about the whole aggregate. If yes, prefer the higher boundary and keep the model simple.

#### Common Mistakes

Do not use composite as a proxy for “this request touches many fields”. That is a modeling shortcut, not a concurrency reason.

#### Architecture Note

Review should reject composites that exist only because a handler touches several nested parts of one aggregate whose invariant is still aggregate-wide.
See [`examples/phase2-parent-over-composite/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-parent-over-composite/README.md) for the teaching example.

### Sync And Async Shared Boundaries

### Shared Versus Split Sync/Async Definitions

#### Problem

A team wants one definition to cover both direct synchronous calls and async worker delivery, and must decide whether `ExecutionKind=both` is really appropriate.

#### Recommended Pattern

Use `ExecutionKind=both` only when the lock boundary, key semantics, and business meaning are truly the same in both paths. Otherwise, split definitions.

#### Recommended Execution Package

Use whichever package matches the call path, but only share the definition when the semantic boundary is genuinely shared.

#### Why This Choice

`ExecutionKind=both` can reduce duplication, but split definitions are safer when sync and async flows have different ownership, idempotency, observability, or review expectations.

#### Example Key Shape

Shared definition example: `order:{order_id}` -> `order:123`  
Example: [`examples/phase2-shared-definition-contention/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-shared-definition-contention/README.md)

#### Best Practices

Ask whether the sync and async flows are protecting the same invariant or just touching the same table row. If they differ in meaning, split definitions.

#### Common Mistakes

Do not reuse one definition across unrelated semantics just because the key looks similar.

#### Architecture Note

Registry review should explicitly ask whether `ExecutionKind=both` preserves one business boundary or hides two different lifecycles behind one name. If the latter, split definitions.
Compare [`examples/phase2-shared-definition-contention/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-shared-definition-contention/README.md) with [`examples/phase2-shared-aggregate-runtime-worker/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-shared-aggregate-runtime-worker/README.md).

### Human Action And Background Worker Touch The Same Aggregate

#### Problem

One aggregate can be touched both by a direct human action path and by a background worker path, and the team needs one coherent boundary without collapsing unlike lifecycles into one definition by default.

#### Recommended Pattern

Use split sync and async definitions over the same aggregate key boundary as the default teaching case.

#### Recommended Execution Package

Use `runtime` for the human action path and `workers` for the background path.

#### Why This Choice

The key boundary can stay shared even when the execution lifecycles are different. Split definitions make that boundary explicit while letting each path keep its own policy surface. One shared `ExecutionKind=both` definition can still be acceptable, but only when both lifecycles really mean the same thing.

#### Example Key Shape

Sync: `order:{order_id}` -> `order:123`  
Async: `order:{order_id}` -> `order:123`  
Example: [`examples/phase2-shared-aggregate-runtime-worker/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-shared-aggregate-runtime-worker/README.md)

#### Best Practices

Keep the key boundary identical if the protected aggregate is the same, but review the sync and async ownership models independently before deciding to share the definition itself.

#### Common Mistakes

Do not force one `ExecutionKind=both` definition just to reduce registry entries. Semantic clarity is worth an extra definition.

#### Architecture Note

This scenario is a governance decision as much as an implementation decision. The registry should optimize for boundary clarity, not for minimum definition count.
See [`examples/phase2-shared-aggregate-runtime-worker/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-shared-aggregate-runtime-worker/README.md) for the split-definition teaching case.

### Lifecycle And Ownership Boundaries

### Inventory Reservation From A Queue Worker

#### Problem

An async consumer processes reservation requests from a queue, and the same logical message may be replayed or redelivered.

#### Recommended Pattern

Use one parent lock for the inventory reservation boundary and pair it with worker-side idempotency.

#### Recommended Execution Package

Use `workers`.

#### Why This Choice

This is not only a locking problem. workers plus idempotency are recommended because async delivery semantics matter as much as locking. The handler must survive duplicate delivery without double-reserving inventory.

#### Example Key Shape

`inventory_item:{sku}` -> `inventory_item:SKU-123`

#### Best Practices

Align the idempotency key with the message identity and keep ownership inside the consumer lifecycle. Make the lock boundary match the inventory invariant you cannot violate.

#### Common Mistakes

Do not treat queue retries as rare edge cases. In async systems, duplicate delivery is normal enough that the lock story is incomplete without idempotency.

#### Architecture Note

Registry review should reject async definitions that clearly need duplicate-delivery protection but omit the idempotency requirement.

### Producer-Consumer Handoff

#### Problem

A producer wants to mark work as “owned” before handing it to a consumer, and the team considers taking a lock in the producer and releasing it in the consumer.

#### Recommended Pattern

Reject that design and move ownership to the consumer claim path.

#### Recommended Execution Package

Use `workers` at the consumer boundary.

#### Why This Choice

Lock in producer, release in consumer is the wrong default because ownership crosses lifecycles and failure domains. Claim ownership begins in the consumer, where delivery, retry, and release decisions actually live.

#### Example Key Shape

`job:{job_id}` -> `job:123`

#### Best Practices

Let the producer emit an intent or message, then let the consumer claim and process it with its own idempotency and lease lifecycle.

#### Common Mistakes

Do not stretch one lease across systems just to “reserve” work early. That usually creates stale ownership and unclear recovery rules.

#### Architecture Note

When teams propose producer-side locking, review should ask which side truly owns retries and completion. In most async systems, that answer is the consumer.

### Shard Or Partition Ownership

### Background Reconciliation Or Shard-Based Batch Job

#### Problem

A queue-triggered reconciliation worker processes batches of data and must decide whether the lock boundary is the whole shard or one batch.

#### Recommended Pattern

Prefer a shard-level parent lock when the invariant is one worker per shard. Prefer a per-batch lock only when each batch is independently safe and replayable.

#### Recommended Execution Package

Use `workers`.

#### Why This Choice

The default example here is queue-triggered background execution, so `workers` is the right package. The decision rule is about the invariant: if one worker per shard is what keeps the system correct, lock the shard. If batches are independently safe and replayable, a narrower per-batch lock is reasonable.

#### Example Key Shape

Shard-level: `reconciliation_shard:{shard_id}` -> `reconciliation_shard:07`  
Batch-level: `reconciliation_batch:{batch_id}` -> `reconciliation_batch:2026-03-27T10`

#### Best Practices

State the invariant explicitly in the definition review. Do not choose the narrower lock shape just because it looks more concurrent on paper.

#### Common Mistakes

Do not use per-batch locking when the real correctness rule is one worker per shard. That turns a safety boundary into an accidental throughput optimization.

#### Architecture Note

This scenario is where governance matters most. Teams should write down why shard-level or batch-level ownership is the real unit of correctness.

### Bulk Import With Shard Ownership

#### Problem

An async bulk import splits work across shards or partitions and needs a default ownership boundary that is safe before the team experiments with narrower units.

#### Recommended Pattern

Prefer shard-level ownership by default.

#### Recommended Execution Package

Use `workers`.

#### Why This Choice

Bulk import usually needs one worker to own one shard at a time so ordering, deduplication, and recovery stay understandable. Smaller batch-level ownership is reasonable only when batches are independently safe and replayable.

#### Example Key Shape

`import-shard:{shard_id}` -> `import-shard:07`  
Example: [`examples/phase2-bulk-import-shard-worker/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-bulk-import-shard-worker/README.md)

#### Best Practices

State the shard or partition invariant first, then size the lock boundary around that invariant instead of around implementation convenience.

#### Common Mistakes

Do not jump to smaller batch locks just because they sound more concurrent. If the real safety rule is shard ownership, keep the lock at the shard.

#### Architecture Note

This is a good place for platform standards. Teams should explain why shard-level or batch-level ownership is the true recovery boundary before registry approval.
See [`examples/phase2-bulk-import-shard-worker/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-bulk-import-shard-worker/README.md) for the default shard-ownership teaching flow.

### Advisory Visibility

### Admin Screen Or Operator Hint

#### Problem

An admin UI or operator tool wants to show whether a resource appears busy before someone clicks an action.

#### Recommended Pattern

Use `CheckPresence` as an advisory signal only.

#### Recommended Execution Package

No execution package is required for the visibility call itself; if the later action must be protected, that action should still use `runtime` or `workers` as appropriate.

#### Why This Choice

`CheckPresence` is useful for visibility, but it must not gate correctness-critical writes. It is an operator hint, not a substitute for acquisition.

#### Example Key Shape

`order:{order_id}` -> `order:123`

#### Best Practices

Tell users the resource appears held instead of promising it is unavailable. Treat the visible status as a hint that can change before the next action starts.

#### Common Mistakes

Do not branch critical business logic on presence alone. That converts an observability feature into a false safety guarantee.

#### Architecture Note

Presence support should be deliberate. Teams should enable it where visibility helps, but review should make clear that correctness still depends on real lock acquisition.

### Migration And Compatibility

### Phase 2a Migration Scenario

#### Problem

A system previously allowed nested parent and child flows because `ParentRef` was treated as metadata more than as enforced runtime policy.

#### Recommended Pattern

Keep the validated parent-child model, but update call paths and expectations to handle explicit overlap rejection.

#### Recommended Execution Package

Use the same package the business flow already belongs to, but assume Phase 2a enforcement now applies to both `runtime` and `workers` when lineage-aware definitions are involved.

#### Why This Choice

After Phase 2a, parent-held child and child-held parent overlap are both rejected. previously permissive flows are now rejected because the lineage policy moved from descriptive metadata into real runtime enforcement.

#### Example Key Shape

Parent: `order:{order_id}` -> `order:123`  
Child: `order:{order_id}:item:{item_id}` -> `order:123:item:line-1`

#### Best Practices

Audit old flows for implicit parent-held child or child-held parent nesting. Update documentation and retry behavior so overlap rejection is handled as a real transient policy outcome.

#### Common Mistakes

Do not assume old nested behavior will keep working just because the exact resource keys differ.

#### Architecture Note

Migration reviews should look for places where teams previously relied on permissive overlap and now need a clearer lock boundary or a composite workflow.

## Best Practices

Treat the sections below as review heuristics and team policy, not as a replacement for the field semantics in [`docs/lock-definition-reference.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-definition-reference.md).

### Central Registry Ownership And Review

Keep one central registry as the source of truth for lock definitions. New definitions should be reviewed by people who understand both the business invariant and the platform rules.

### Stable, Business-Readable Definition IDs

Use names that describe the business boundary, not the implementation detail. `OrderApprovalLock` or `TransferComposite` is easier to govern than vague IDs tied to storage layout.

### Template Key Builders And Explicit Fields

Prefer template key builders so required fields are obvious in review. If a key shape is hard to explain in one sentence, the lock boundary may already be too muddy.

### TTL Sizing Guidance

Set TTLs long enough for the real unit of work plus jitter and shutdown delay, but short enough that stale ownership does not linger forever after crashes. Review should ask what failure window the team is accepting.

### Keep Composite Size Small

Composites should remain small and operation-specific. Large composites often mean the business operation is under-modeled or the team is compensating for unclear boundaries with “lock more things”.

### Avoid Nested Manual Lock Orchestration

If one flow needs several resources, prefer composite execution over hand-written acquire chains. Nested manual orchestration leaks ordering, rollback, and review complexity into application code.

### Prefer Parent Locks When Aggregate Invariants Dominate

Start from the aggregate boundary. Move to child locks only when the team can clearly explain the concurrency benefit and why aggregate-wide serialization is not the right answer.

### Use Child Locks Only For Intentional Sub-Resource Concurrency

Child definitions should exist because independent sub-resource concurrency is a real product requirement, not because the domain happens to have nested IDs.

### Treat CheckPresence As Advisory Only

`CheckPresence` is useful for visibility and operator experience, but it must never become the correctness boundary for writes or async claims.

### Keep Async Idempotency Aligned With Message Ownership

When a queue consumer owns retries and completion, the idempotency key and lease lifecycle should begin and end in that consumer path. Do not split those concerns across producer and consumer.

### Distinguish Overlap Rejection From Lock Busy

Exact-key contention and parent-child overlap are different failures. Application code and operational runbooks should not collapse them into one generic “lock busy” bucket.

### Choose ExecutionKind=both Only When The Boundary Is Truly Shared

`ExecutionKind=both` is appropriate only when sync and async flows are protecting the same business boundary. If they differ in lifecycle or meaning, split definitions and make the review explicit.

## Anti-Patterns

### Too Many Children Where One Parent Is Simpler

This adds lineage complexity without buying safe parallelism. Prefer one parent lock when the invariant is still aggregate-wide.

### One Coarse Parent Where Sub-Resource Concurrency Is Required

This turns a valid concurrency opportunity into needless serialization. Prefer child locks only when the sub-resource operations are truly independent.

### Composite Where One Parent Lock Is Enough

If one parent definition already captures the real invariant, a composite is just a more complicated spelling of the same boundary.

### Manual Multi-Lock Orchestration In Application Code

Application code should not reinvent ordering, rollback, and partial-failure handling. Use composite definitions instead.

### Producer Acquire And Consumer Release

This spreads one ownership lifecycle across two systems. Let the consumer claim and release work inside its own retry and idempotency model.

### Presence As A Correctness Signal

Do not branch correctness-critical writes on presence checks. Acquire the lock or claim the work for real.

### One Definition Reused Across Unrelated Semantics

Shared keys do not automatically mean shared business meaning. If two flows have different lifecycles or governance rules, they should not hide behind one definition.

### Pre-Phase-2a Assumptions About ParentRef

Do not assume `ParentRef` is metadata only. After Phase 2a, parent-child overlap rules are enforced and old permissive nesting may fail.

## Decision Matrix

| Scenario Type | Recommended Lock Shape | Package | Why | Next Doc/Example |
|---|---|---|---|---|
| Approve one order | Parent lock | `runtime` | One aggregate transition, one caller waiting now | [`docs/runtime-vs-workers.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/runtime-vs-workers.md) |
| Update one order item under order-level invariants | Child lock with lineage, or parent if aggregate-wide | `runtime` or `workers` | Choose child only when independent item concurrency is intentional | [`examples/phase2-parent-child-runtime/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-parent-child-runtime/README.md) |
| Transfer between two accounts | Composite lock | `runtime` or `workers` | Multi-resource coordination should stay inside the SDK | [`examples/phase2-composite-sync/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-composite-sync/README.md) |
| Inventory reservation from a queue worker | Parent lock plus idempotency | `workers` | Async delivery semantics matter as much as locking | [`examples/phase2-basic/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-basic/README.md) |
| Background reconciliation or shard-based batch job | Shard lock by default, batch lock only if independently safe and replayable | `workers` | The invariant decides whether shard or batch owns correctness | [`docs/runtime-vs-workers.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/runtime-vs-workers.md) |
| Producer-consumer handoff | Consumer-side claim ownership | `workers` | Ownership should begin where retry and completion are actually decided | [`examples/phase2-basic/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-basic/README.md) |
| Admin screen or operator hint | Presence-check-only usage | none for the check itself | Visibility is useful, but it is not a correctness gate | [`docs/lock-definition-reference.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-definition-reference.md) |
| Phase 2a migration scenario | Validated parent-child model with explicit overlap handling | same as the flow | Old permissive nesting no longer survives Phase 2a | [`examples/phase2-parent-child-runtime/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-parent-child-runtime/README.md) |
| Shared versus split sync/async definitions | `ExecutionKind=both` only when semantics are truly shared | depends on the flow | Shared keys are not enough; shared meaning is required | [`examples/phase2-shared-definition-contention/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-shared-definition-contention/README.md) |
| Human action and background worker touch the same aggregate | Split sync and async definitions over one shared aggregate key boundary | `runtime` and `workers` | Shared key boundary does not force one shared definition | [`examples/phase2-shared-aggregate-runtime-worker/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-shared-aggregate-runtime-worker/README.md) |
| One higher aggregate parent lock is enough, composite is overkill | Parent lock | `runtime` | Aggregate-wide invariant is already captured without composite | [`examples/phase2-parent-over-composite/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-parent-over-composite/README.md) |
| Bulk import with shard ownership | Shard-level ownership by default | `workers` | Shard ownership is the safer default recovery boundary | [`examples/phase2-bulk-import-shard-worker/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-bulk-import-shard-worker/README.md) |

## Related Docs And Examples

- Execution package choice: [`docs/runtime-vs-workers.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/runtime-vs-workers.md)
- Definition field semantics: [`docs/lock-definition-reference.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-definition-reference.md)
- Single-resource worker example: [`examples/phase2-basic/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-basic/README.md)
- Sync composite example: [`examples/phase2-composite-sync/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-composite-sync/README.md)
- Async composite example: [`examples/phase2-composite-worker/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-composite-worker/README.md)
- Reject-first overlap example: [`examples/phase2-overlap-reject/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-overlap-reject/README.md)
- Phase 2a parent-child runtime example: [`examples/phase2-parent-child-runtime/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-parent-child-runtime/README.md)
- Shared aggregate runtime/worker example: [`examples/phase2-shared-aggregate-runtime-worker/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-shared-aggregate-runtime-worker/README.md)
- Shared definition contention example: [`examples/phase2-shared-definition-contention/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-shared-definition-contention/README.md)
- Parent over composite example: [`examples/phase2-parent-over-composite/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-parent-over-composite/README.md)
- Bulk import shard worker example: [`examples/phase2-bulk-import-shard-worker/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-bulk-import-shard-worker/README.md)
