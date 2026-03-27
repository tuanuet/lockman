# Lock Scenarios And Best Practices Design

## Summary

Expand the scenario guidance work into a combined docs-and-examples increment.

The repository should:

- regroup [`docs/lock-scenarios-and-best-practices.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-scenarios-and-best-practices.md) around scenario families instead of a flat scenario list
- keep the existing scenario guidance, but rebalance it so old and new scenarios read as one coherent handbook
- add three new runnable examples that match the new scenarios and can be used as teaching material alongside the guide

The document should serve two audiences at once:

- application engineers choosing how to protect one concrete flow
- architects and tech leads defining registry conventions, governance, and platform guidance

The guide should be deeper than a quickstart. It should not just say "use runtime for sync and workers for async". It should explain why one pattern is a good fit, what trade-offs it carries, what failure modes to watch for, and what best practices should become team standards.

## Why This Document Is Needed

The repository already has:

- [`docs/runtime-vs-workers.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/runtime-vs-workers.md) for choosing execution packages
- [`docs/lock-definition-reference.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-definition-reference.md) for public field semantics
- runnable examples in [`examples/`](/Users/mrt/workspaces/boilerplate/lockman/examples) for concrete API usage

What is still missing is scenario-driven guidance.

Teams still need help answering questions like:

- should this flow use one parent lock or several child locks?
- when is a composite the right answer and when is it overkill?
- when should a producer avoid taking a lock at all?
- when does `CheckPresence` help, and when is it dangerous?
- what changed after Phase 2a and what old patterns are now wrong?

This guide fills that gap.

It should stay scenario-driven and governance-driven. When it needs to mention package choice or field semantics, it should summarize briefly and link out rather than re-teaching the full reference material.

The current example suite also misses several realistic boundaries that teams keep asking about:

- one aggregate touched by both human-triggered sync actions and background workers
- cases where a team reaches for composite but one higher aggregate lock is actually enough
- shard or partition ownership for bulk import and large async workloads

Those gaps should now be filled through additional runnable examples.

## Goals

- Provide practical lock-pattern guidance for real business scenarios.
- Help teams choose between parent, child, composite, runtime, workers, and presence-check-only usage.
- Make Phase 2a behavior clear in business terms, especially parent-child overlap enforcement.
- Capture best practices and anti-patterns that should become team conventions.
- Give architects enough governance notes to standardize registry design and review pull requests consistently.
- Add runnable examples for the new scenarios so the guide links to concrete teaching artifacts, not prose alone.

## Non-Goals

- Do not redefine public API contracts already covered in [`docs/lock-definition-reference.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-definition-reference.md).
- Do not duplicate the full runtime-versus-workers explanation already covered in [`docs/runtime-vs-workers.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/runtime-vs-workers.md).
- Do not introduce new SDK behavior or new lock kinds.
- Do not become a backend operations manual for Redis or any other driver.
- Do not turn the new examples into mini applications; they should stay focused teaching examples with deterministic output.

## Target Audience

### Application Engineers

Need concrete guidance for one flow they are implementing now:

- what lock shape should I use?
- which package should I call?
- what key should I build?
- what mistakes should I avoid?

### Architects And Tech Leads

Need governance guidance across many flows:

- how should teams name and scope definitions?
- when should registry review reject a proposed lock shape?
- when should a team use one shared parent lock instead of many child locks?
- how should Phase 2a migration be communicated?

## Document Shape

The guide remains one standalone file:

- [`docs/lock-scenarios-and-best-practices.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-scenarios-and-best-practices.md)

But it should no longer read like one flat scenario list. It should be regrouped around scenario families so old and new scenarios feel balanced.

The new guide should use this high-level shape:

1. `Who This Guide Is For`
2. `Decision Framing`
3. `Quick Decision Guide`
4. `Pattern Catalog`
5. `Scenario Families`
6. `Best Practices`
7. `Anti-Patterns`
8. `Decision Matrix`
9. `Related Docs And Examples`

## Section 1: Purpose And Decision Framing

Open with a short explanation of what problem this guide solves.

Define the main distinctions that often get confused:

- ordinary lock contention
- parent-child overlap rejection
- same-process reentrant rejection
- duplicate-delivery/idempotency concerns
- advisory presence checks versus correctness guarantees

This section should make the rest of the guide easier to read by giving teams a shared vocabulary.

## Section 2: Quick Decision Guide

Provide a fast path before the deep sections.

This should answer:

- direct caller waiting now -> prefer `runtime`
- queue delivery or replayable async job -> prefer `workers`
- one aggregate-level invariant -> prefer one parent lock
- independently processable sub-resources with aggregate boundary -> consider child locks
- one operation needs multiple independent resources together -> consider composite
- only need UI/operator hint -> consider presence check only
- if correctness depends on presence alone -> stop and redesign

This section should be short and skimmable.

It should not become a second version of [`docs/runtime-vs-workers.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/runtime-vs-workers.md). It should give scenario-specific direction in one or two lines, then link readers to that guide for the full package-choice discussion.

## Section 3: Pattern Catalog

Describe the core patterns in a pattern-first structure.

Required patterns:

- parent lock
- child lock
- composite lock
- presence-check-only usage

Each pattern should use the same subsection layout:

- what it is
- when it is a good fit
- when it is a bad fit
- trade-offs
- architecture and registry notes

The goal is consistency, so readers can compare patterns quickly.

`runtime` and `workers` should not appear here as standalone catalog entries. They should be referenced inside scenario recommendations and in the quick decision guide with links to the dedicated execution-package document.

## Section 4: Scenario Families

This remains the heart of the document, but it should now be regrouped by family instead of presented as one flat list.

Required family groupings:

- single aggregate ownership
- aggregate versus sub-resource concurrency
- multi-resource coordination
- sync and async shared boundaries
- lifecycle and ownership boundaries
- shard or partition ownership
- advisory visibility
- migration and compatibility

Each family can contain one or more scenarios. The goal is to make the reader compare related design choices next to each other.

The implementation should assign every required scenario to one of those families explicitly, so the regrouped structure is deterministic rather than interpretive.

Each scenario should still include:

- problem statement
- recommended pattern
- recommended execution package
- why this choice is preferred
- example key shape
- best practices
- common mistakes
- architecture note

Required scenarios:

1. **Approve one order**
   - one aggregate lock
   - sync path
   - use parent lock through `runtime`

2. **Update one order item under order-level invariants**
   - child lock with parent lineage
   - explain when child is justified and when parent-only is simpler
   - call out Phase 2a overlap behavior

3. **Transfer between two accounts**
   - composite lock
   - explain why two separate manual acquires are inferior

4. **Inventory reservation from a queue worker**
   - worker flow with idempotency
   - explain why async delivery semantics matter as much as locking

5. **Background reconciliation or shard-based batch job**
   - prefer shard lock when the invariant is "only one worker may process this shard at a time"
   - prefer per-batch lock only when individual batches are independently safe and replayable
   - assume the default example is queue-triggered batch execution, so the default recommendation should be `workers`
   - explain the invariant that separates shard-level from batch-level locking

6. **Producer-consumer handoff**
   - explicitly explain why "lock in producer, release in consumer" is usually the wrong design
   - recommend claim ownership beginning in the consumer instead

7. **Admin screen or operator hint**
   - `CheckPresence`
   - explain advisory-only semantics and why it must not gate correctness-critical writes

8. **Phase 2a migration scenario**
   - parent-held child request and child-held parent request
   - explain what previously permissive flows are now rejected

9. **Shared versus split sync/async definitions**
   - explain when `ExecutionKind=both` is acceptable
   - explain when separate sync and async definitions are safer
   - include governance guidance for registry review

10. **Human action and background worker touch the same aggregate**
   - explain one aggregate boundary shared across sync and async lifecycles
   - show when one shared key boundary exists but the lifecycle difference still argues for split definitions
   - treat split sync and async definitions over the same key boundary as the default teaching case
   - explain in prose when one shared `ExecutionKind=both` definition would still be acceptable
   - tie the prose back to a runnable example

11. **One higher aggregate parent lock is enough, composite is overkill**
   - explain a case where a team might reach for composite because several sub-resources are involved, but one higher aggregate parent lock already captures the real invariant
   - make the “do not over-model with composite” guidance concrete
   - tie the prose back to a runnable example

12. **Bulk import with shard ownership**
   - explain shard or partition ownership for a large async import workload
   - compare shard-level ownership with smaller batch-level ownership
   - tie the prose back to a runnable example

## Example Additions

This scope now includes three new teaching examples.

### Example 1: Shared Aggregate Across Runtime And Workers

- Create a new Redis-backed example:
  [`examples/phase2-shared-aggregate-runtime-worker`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-shared-aggregate-runtime-worker)
- The example should show one aggregate touched by a direct human action path and a background worker path.
- The runnable example should use split sync and async definitions over the same aggregate key boundary as the recommended teaching case.
- Its README should also explain when a single shared `ExecutionKind=both` definition would be acceptable, but the example body should not try to demonstrate both designs at once.
- It should teach boundary choice more than low-level mechanics.
- It should stay balanced in complexity: enough output to explain the flow, but not a mini application.

### Example 2: Parent Over Composite

- Create a new memory-backed example:
  [`examples/phase2-parent-over-composite`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-parent-over-composite)
- The example should show that one higher aggregate parent lock can be the right answer even when several sub-resources inside the same business aggregate are involved.
- It should explicitly teach why composite would be overkill in this case.

### Example 3: Bulk Import With Shard Ownership

- Create a new Redis-backed worker example:
  [`examples/phase2-bulk-import-shard-worker`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-bulk-import-shard-worker)
- The example should show shard ownership for async bulk import or large partitioned work.
- It should make the shard-level boundary obvious in both output and README guidance.

### Example Style Requirements

All three new examples should:

- follow the existing example pattern of `main.go`, `main_test.go`, and local `README.md`
- produce deterministic teaching output rather than logs that depend on timing races
- explain one primary lesson each
- keep tests aligned with the printed output contract
- link cleanly from the regrouped guide

## Section 5: Best Practices

This section should generalize rules that appear across multiple scenarios.

It should be explicitly bounded as team heuristics and review policy, not a second field-by-field reference. When it mentions IDs, key builders, TTLs, or execution kind, it should focus on review guidance and link to [`docs/lock-definition-reference.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-definition-reference.md) for the underlying field semantics.

Required topics:

- central registry ownership and review
- stable, business-readable definition IDs
- template key builders and explicit field requirements
- TTL sizing guidance
- keep composite size small
- avoid nested manual lock orchestration
- prefer parent lock when aggregate invariants dominate
- use child locks only when independent sub-resource concurrency is intentional
- treat `CheckPresence` as advisory only
- keep async idempotency aligned with message ownership
- distinguish overlap rejection from lock busy in app logic
- when `ExecutionKind=both` is appropriate versus when sync and async definitions should be split

This section should read like recommendations teams can copy into internal standards.

## Section 6: Anti-Patterns

Make the bad patterns explicit.

Required anti-patterns:

- using many children where one parent lock would be simpler and safer
- using one coarse parent lock where sub-resource parallelism is actually required
- using composite when a single parent lock already expresses the invariant
- manually acquiring multiple locks in application code instead of using composite
- locking in producer and releasing in consumer
- treating presence checks as a correctness signal
- sharing one lock definition across unrelated business semantics
- relying on pre-Phase-2a assumptions that parent-child overlap is metadata only

Each anti-pattern should explain why it is risky and what to do instead.

## Section 7: Decision Matrix And Related Docs

Close with a compact table:

- scenario type
- recommended lock shape
- package
- why
- docs/example to read next

Then link readers to:

- [`docs/runtime-vs-workers.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/runtime-vs-workers.md)
- [`docs/lock-definition-reference.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-definition-reference.md)
- [`examples/phase2-basic/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-basic/README.md)
- [`examples/phase2-composite-sync/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-composite-sync/README.md)
- [`examples/phase2-composite-worker/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-composite-worker/README.md)
- [`examples/phase2-overlap-reject/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-overlap-reject/README.md)
- [`examples/phase2-parent-child-runtime/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-parent-child-runtime/README.md)
- [`examples/phase2-shared-aggregate-runtime-worker/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-shared-aggregate-runtime-worker/README.md)
- [`examples/phase2-parent-over-composite/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-parent-over-composite/README.md)
- [`examples/phase2-bulk-import-shard-worker/README.md`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-bulk-import-shard-worker/README.md)

## Tone And Style

The guide should be practical and opinionated.

It should:

- prefer clear recommendations over vague possibility lists
- explain trade-offs without becoming theoretical
- stay close to real product design decisions
- avoid backend-specific detail unless it changes the advice materially
- use short examples of key shapes and flow shapes where useful

It should not:

- read like an API reference
- repeat long code snippets from examples
- overload readers with implementation detail from internal packages

## Relationship To Existing Docs

This document should become the "how do I choose and apply the right lock shape?" guide.

Recommended navigation model:

- `README.md`: high-level entry point
- `docs/lock-scenarios-and-best-practices.md`: scenario and governance guide
- `docs/runtime-vs-workers.md`: execution package choice
- `docs/lock-definition-reference.md`: field-by-field contract reference
- `examples/*/README.md`: runnable API usage

## Acceptance Criteria

The design is satisfied when the implementation:

- exists at [`docs/lock-scenarios-and-best-practices.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-scenarios-and-best-practices.md)
- keeps the opening purpose and target-audience framing
- contains a short purpose section that distinguishes contention, overlap, reentrancy, idempotency, and advisory presence
- contains a quick decision guide that gives scenario-specific package direction and links to [`docs/runtime-vs-workers.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/runtime-vs-workers.md) instead of duplicating it
- contains a pattern catalog for parent, child, composite, and presence-check-only usage
- groups the real scenarios into the required scenario families rather than leaving them as one flat list
- assigns each required scenario to one explicit scenario family
- contains both scenario guidance and architecture best practices
- clearly explains Phase 2a migration impact
- gives concrete recommendations for all twelve required scenarios
- gives each required scenario the full structure of problem, recommended pattern, recommended execution package, why, example key shape, best practices, common mistakes, and architecture note
- contains a bounded best-practices section framed as review heuristics and team policy
- contains an explicit anti-pattern section covering the required bad patterns
- contains a compact decision matrix with scenario type, recommended lock shape, package, why, and next doc/example
- links to the existing runtime/workers guide, definition reference, and both the old and new examples
- adds the three new example directories:
  - [`examples/phase2-shared-aggregate-runtime-worker`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-shared-aggregate-runtime-worker)
  - [`examples/phase2-parent-over-composite`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-parent-over-composite)
  - [`examples/phase2-bulk-import-shard-worker`](/Users/mrt/workspaces/boilerplate/lockman/examples/phase2-bulk-import-shard-worker)
- gives each new example a `main.go`, `main_test.go`, and `README.md`
- maps the new examples to the intended backend shape:
  - shared aggregate runtime/worker -> Redis-backed
  - parent over composite -> memory-backed
  - bulk import with shard ownership -> Redis-backed worker
- ensures each new example has one primary lesson, deterministic teaching output, and a `main_test.go` that validates the output contract
- ensures the regrouped guide links to each new example README in the final related-docs/examples area and in the relevant scenario rows of the decision matrix
- avoids duplicating large sections of existing docs verbatim
