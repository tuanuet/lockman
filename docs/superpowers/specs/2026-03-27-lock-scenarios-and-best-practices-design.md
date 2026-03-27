# Lock Scenarios And Best Practices Design

## Summary

Add a new user-facing guide at [`docs/lock-scenarios-and-best-practices.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-scenarios-and-best-practices.md) that explains how to choose lock patterns for real product scenarios and how to apply them safely.

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

## Goals

- Provide practical lock-pattern guidance for real business scenarios.
- Help teams choose between parent, child, composite, runtime, workers, and presence-check-only usage.
- Make Phase 2a behavior clear in business terms, especially parent-child overlap enforcement.
- Capture best practices and anti-patterns that should become team conventions.
- Give architects enough governance notes to standardize registry design and review pull requests consistently.

## Non-Goals

- Do not redefine public API contracts already covered in [`docs/lock-definition-reference.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-definition-reference.md).
- Do not duplicate the full runtime-versus-workers explanation already covered in [`docs/runtime-vs-workers.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/runtime-vs-workers.md).
- Do not introduce new SDK behavior, new lock kinds, or new examples.
- Do not become a backend operations manual for Redis or any other driver.

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

The new guide should be one standalone file:

- [`docs/lock-scenarios-and-best-practices.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-scenarios-and-best-practices.md)

It should be structured in seven major sections.

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

## Section 3: Pattern Catalog

Describe the core patterns in a pattern-first structure.

Required patterns:

- parent lock
- child lock
- composite lock
- runtime execution
- worker execution
- presence-check-only usage

Each pattern should use the same subsection layout:

- what it is
- when it is a good fit
- when it is a bad fit
- trade-offs
- architecture and registry notes

The goal is consistency, so readers can compare patterns quickly.

## Section 4: Real Scenarios

This is the heart of the document.

Each scenario should include:

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
   - parent-like shard lock or per-batch lock
   - explain lock granularity trade-offs

6. **Producer-consumer handoff**
   - explicitly explain why "lock in producer, release in consumer" is usually the wrong design
   - recommend claim ownership beginning in the consumer instead

7. **Admin screen or operator hint**
   - `CheckPresence`
   - explain advisory-only semantics and why it must not gate correctness-critical writes

8. **Phase 2a migration scenario**
   - parent-held child request and child-held parent request
   - explain what previously permissive flows are now rejected

## Section 5: Best Practices

This section should generalize rules that appear across multiple scenarios.

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
- the Phase 2a example guides under [`examples/`](/Users/mrt/workspaces/boilerplate/lockman/examples)

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

The design is satisfied when the implemented document:

- exists at [`docs/lock-scenarios-and-best-practices.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-scenarios-and-best-practices.md)
- contains both scenario guidance and architecture best practices
- clearly explains Phase 2a migration impact
- gives concrete recommendations for at least the eight required scenarios
- links to the existing runtime/workers guide, definition reference, and examples
- avoids duplicating large sections of existing docs verbatim
