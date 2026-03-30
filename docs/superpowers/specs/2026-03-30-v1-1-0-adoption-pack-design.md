# v1.1.0 Adoption Pack Design

## Status

Approved

## Goal

Shape `v1.1.0` as an adoption-focused release for application teams that want to use `lockman` safely without studying internal engine details or inferring policy from multiple separate docs.

The release should reduce ambiguity first and add benchmark evidence second.

## Audience

Primary audience:

- application teams integrating `lockman` into one service at a time

Not the primary audience:

- platform teams defining cross-service coordination standards
- contributors exploring new lock primitives

## Why This Release Exists

The repository already has:

- stable root and nested module releases
- example-driven quickstarts
- advanced docs for strict, composite, lineage, and guard flows
- CI and release automation

What it still lacks for adoption is:

- one obvious production-facing guide that tells application teams what to choose
- a small benchmark suite that quantifies the cost of the main choices

Without those two things, a new team still has to answer too many questions on its own:

- Should this flow use `Run` or `Claim`?
- Do we actually need `strict`?
- Is `composite` the right shape, or is one aggregate lock enough?
- What is the overhead of taking the advanced path?

## Release Strategy

This release is `guidance-first with benchmark support`.

That means:

- the main artifact is a production guide for application teams
- benchmarks exist to support the guide, not replace it
- no new locking primitives are required for the release to be successful

## Non-Goals

This release should not:

- introduce new coordination primitives just to justify a minor version
- benchmark every internal operation or driver behavior
- try to become a platform architecture manual
- optimize for synthetic microbenchmarks with no user-facing decision value

## Deliverables

### 1. Production Guide

Add one new doc as the main adoption entry point for application teams:

- `docs/production-guide.md`

This document should answer direct adoption questions, not just explain APIs.

Recommended sections:

1. `Start Here`
2. `Choose Run Or Claim`
3. `Minimum Production Wiring`
4. `Stay On The Default Path`
5. `When Strict Is Worth It`
6. `When Composite Is Worth It`
7. `TTL And Renewal Mindset`
8. `Identity And Ownership`
9. `Production Checklist`
10. `Common Mistakes`
11. `Which Example To Copy`

This guide should explicitly tell teams when *not* to use advanced features.

The `Minimum Production Wiring` section should explicitly cover:

- backend selection and current supported adapter paths
- why `Claim` implies idempotency wiring from the start
- what startup registration and capability mismatches should fail fast
- the minimum root SDK wiring an application team should copy first

### 2. Benchmark Suite

Add a small benchmark suite in the repo that answers the decisions raised in the guide.

Benchmark coverage should include:

- `Run` uncontended path
- `Run` under same-key contention
- `Claim` uncontended path
- `Claim` duplicate-delivery or retry-style contention path
- default path versus `strict`
- `composite` with increasing member counts such as `1`, `2`, and `4`
- renewal-heavy flow with short TTL to show long-running execution cost

The benchmark suite should aim for comparison value, not exhaustive runtime modeling.

The suite must include both:

- a tightly controlled baseline track for stable relative comparisons
- at least one adapter-backed track that reflects the real adoption path through the published Redis-backed modules

The adapter-backed track exists so benchmark conclusions stay grounded in the production path application teams will actually wire.

### 3. Benchmark Report

Add one documentation page that explains:

- what was measured
- how to run the benchmarks
- how to interpret the numbers
- what conclusions application teams should and should not draw

Recommended file:

- `docs/benchmarks.md`

This doc should emphasize relative overhead and contention shape rather than absolute claims that only hold on one machine.

### 4. README Integration

Update the root README so an application team can discover:

- the production guide
- the benchmark methodology and latest report

These links should sit near the existing quickstart and advanced decision docs.

## Documentation Positioning

The new production guide should become the preferred answer for:

- "How do I adopt this in a normal service?"
- "What should I choose first?"
- "Do I really need the advanced path?"

The existing docs should remain as supporting references:

- `docs/runtime-vs-workers.md` remains the short `Run` versus `Claim` concept page
- `docs/quickstart-sync.md` and `docs/quickstart-async.md` remain setup-oriented quickstarts
- advanced docs remain deeper references once a team already knows it needs that path

## Benchmark Design Principles

The benchmark suite should follow these rules:

- measure application-facing choices, not random internal helpers
- keep scenarios small and reproducible
- keep one tightly controlled baseline harness for stable comparison
- include at least one adapter-backed harness for production-path relevance
- separate uncontended and contended cases
- make advanced-path overhead visible against the default path baseline
- document environment assumptions beside the results

The suite does not need to prove that one driver is globally fast. It needs to clarify cost shape.

## Expected User Questions And How This Release Answers Them

### Question: Should I use `Run` or `Claim`?

Answer via:

- production guide decision section
- benchmark comparison showing the additional machinery on the `Claim` path

### Question: Should I start with `strict`?

Answer via:

- production guide default-path guidance
- benchmark comparison of default versus strict path overhead

### Question: Should I use `composite`?

Answer via:

- production guide examples and anti-patterns
- benchmark comparison by composite member count

### Question: Is the default path light enough for request/response work?

Answer via:

- `Run` uncontended benchmark
- short interpretation guidance in benchmark docs

## Success Criteria

`v1.1.0` succeeds if a new application team can quickly answer all of the following:

- Which flows should use `Run`?
- Which flows should use `Claim`?
- When should we avoid `strict` and `composite`?
- What overhead comes with the advanced paths?
- Which repository example is the right starting point?

Concrete acceptance checks:

- `docs/production-guide.md` exists and contains explicit sections for `Run` versus `Claim`, minimum production wiring, advanced-path avoidance guidance, a production checklist, and example selection guidance
- `docs/benchmarks.md` exists and documents benchmark command(s), environment assumptions, and interpretation guidance
- the benchmark suite contains the planned comparison cases and can be invoked from one documented command path
- the README links directly to the production guide and benchmark doc
- the production guide explicitly states that `Claim` requires idempotency wiring and shows the minimum startup wiring shape for application teams

## Implementation Shape

The implementation should be split into two tracks:

### Track A: Guidance

- write the production guide
- link it from the README
- refine existing docs only where they conflict or create ambiguity

### Track B: Benchmarks

- add the benchmark harness and cases
- document the benchmark command
- publish the first benchmark interpretation doc

Track A is the primary objective. Track B exists to support Track A with evidence.

## Risks

### Risk: Benchmark Results Get Over-Interpreted

Mitigation:

- document environment and scope clearly
- present results as comparisons and tendencies
- avoid broad claims like "production-ready for all workloads"

### Risk: The Production Guide Repeats Existing Docs Without Clarifying Choices

Mitigation:

- write it as a decision guide, not a tutorial clone
- bias toward concrete recommendations and anti-recommendations

### Risk: Scope Expands Into Feature Work

Mitigation:

- reject new primitives from this release unless a benchmark or guide task proves an actual gap

## Recommended Next Step

Write the implementation plan for:

- `docs/production-guide.md`
- `docs/benchmarks.md`
- the benchmark suite and harness files
- README integration
