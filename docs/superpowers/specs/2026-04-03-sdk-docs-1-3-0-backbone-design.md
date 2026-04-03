# SDK Docs 1.3.0 Backbone Design

## Status

Approved for specification drafting

## Goal

Reshape the root README and the examples documentation so `v1.3.0` becomes the narrative backbone of the SDK.

The documentation should teach `LockDefinition[T]` and definition-first authoring as the primary mental model, while treating shorthand constructors such as `DefineRun`, `DefineHold`, and `DefineClaim` as convenience forms for narrower cases.

## Problem Statement

The repository already introduced the `v1.3.0` definition-first model in code and changelog guidance:

1. `DefineLock` defines shared lock identity
2. `DefineRunOn`, `DefineHoldOn`, and `DefineClaimOn` attach execution surfaces to one shared definition
3. shared definitions now carry core SDK semantics such as shared lock identity and definition-level strictness

But the public documentation still presents an older center of gravity:

1. the root README opens with `DefineRun` as the primary happy path
2. the examples index reads more like a flat list than a guided learning path
3. readers can discover shared definitions, but they are not clearly taught as the main authoring model
4. `examples/core` and `examples/sdk` are both visible, but the distinction is not strong enough to keep the SDK story centered on the public path

This creates an adoption mismatch. The code says `v1.3.0` is definition-first, but the docs still feel use-case-first.

## Audience

Primary audience:

1. application engineers adopting the root SDK path `github.com/tuanuet/lockman`
2. readers evaluating how to model one business resource across sync, async, and hold flows
3. contributors looking for the canonical examples that represent the current public interface

Secondary audience:

1. maintainers updating docs and examples in future releases
2. teams migrating from shorthand use cases to shared definitions

Not the primary audience:

1. readers exploring low-level engine semantics before they understand the public SDK path
2. readers looking for advanced scenario coverage before they know the default model

## Desired Narrative Shift

The documentation should teach the SDK in this order:

1. define lock identity first with `DefineLock`
2. attach one or more execution surfaces with `DefineRunOn`, `DefineHoldOn`, or `DefineClaimOn`
3. register those use cases and execute them through the root client
4. use shorthand constructors only when a dedicated private definition is sufficient

This means `LockDefinition[T]` becomes the conceptual backbone of the SDK docs, not just an advanced feature section.

The rewritten docs should also make this historical framing explicit at least once in the root README and once in `examples/README.md`:

1. `v1.3.0` introduced the definition-first shared-lock authoring model
2. the README and SDK examples now treat that model as the primary public path

The goal is not a migration guide. The goal is to anchor the documentation story to the `v1.3.0` model instead of leaving that shift implicit.

## Core Documentation Positioning

### Root README

The root README should become the strongest statement of the public SDK model.

It should no longer imply that the main story starts with `DefineRun` plus inline binding. Instead, it should teach that:

1. `DefineLock` is the reusable lock contract for a resource domain
2. execution surfaces are attached to that contract
3. shared definitions are the normal way to express one business boundary with multiple coordination modes
4. shorthand constructors exist as ergonomic shortcuts, not as the primary teaching path

### Examples Index

`examples/README.md` should become a learning map, not just a file inventory.

It should guide the reader through:

1. one canonical starting example
2. the next examples to read depending on whether they need a sync or async execution surface after learning the shared-definition backbone
3. when to stay in `examples/sdk`
4. when to drop down into `examples/core` for preserved lower-level teaching material

### Example-Level READMEs

The canonical SDK examples should repeat the same vocabulary and teaching structure.

Each README should make it easy to answer:

1. what definition is being created
2. which execution surface is attached to it
3. why this example is on the SDK path
4. whether the example demonstrates the default backbone or an advanced variation

## Scope

In scope:

1. rewrite the root `README.md` to use a definition-first narrative
2. rewrite `examples/README.md` as a guided SDK learning map
3. align the required canonical SDK example READMEs with the same vocabulary and positioning
4. clarify the role of `examples/sdk` versus `examples/core`
5. update any status or cross-links in README content that conflict with the `v1.3.0` narrative

Out of scope:

1. changing public APIs or runtime behavior
2. restructuring or renaming the directory tree
3. rewriting every example README in the repository if only the canonical SDK path needs alignment first
4. changing lower-level `examples/core` scenario code unless a small doc alignment requires it

## Required Files

The implementation must update exactly these documentation entry points for acceptance:

1. `README.md`
2. `examples/README.md`
3. `examples/sdk/shared-lock-definition/README.md`
4. `examples/sdk/sync-approve-order/README.md`
5. `examples/sdk/async-process-order/README.md`
6. `examples/sdk/shared-aggregate-split-definitions/README.md`
7. `examples/sdk/parent-lock-over-composite/README.md`
8. `examples/sdk/sync-fenced-write/README.md`

Additional README updates are allowed only if both of the following are true:

1. the file is directly linked from one of the required files above
2. leaving it untouched would create an immediate contradiction about the definition-first backbone

This exception is limited to one hop from the required files above and must not be used to expand the rewrite into a broader documentation sweep.

## Definitions For Review

For this spec, the following terms are normative:

1. `first substantial authoring code sample` means the first multi-line Go code block in a README that shows SDK authoring or execution, not a one-line import, install command, or isolated type declaration
2. `aligned vocabulary` means explicitly using both `lock definition` and either `execution surface` or a named surface constructor such as `DefineRunOn`, `DefineHoldOn`, or `DefineClaimOn`
3. `clearly positioned` means the text explicitly says new SDK readers should start in `examples/sdk`, while `examples/core` is presented afterward as preserved deeper or lower-level material

## Recommended Documentation Structure

### 1. Root README Structure

Recommended sections:

1. `Why lockman`
2. `The SDK Backbone`
3. `Definition-First Happy Path`
4. `When Shorthand Is Enough`
5. `Examples By Learning Path`
6. `Run, Hold, Or Claim?`
7. `When You Need More`
8. `Status`
9. `Development`

These sections are not just illustrative. The rewritten root README must satisfy all of the following ordering rules:

1. `The SDK Backbone` appears before the first substantial code sample
2. `Definition-First Happy Path` contains the first substantial code sample in the file
3. `When Shorthand Is Enough` appears only after the definition-first happy path has already been established

The root README should also include one short sentence in either `The SDK Backbone` or immediately before it that explicitly ties this narrative to the `v1.3.0` definition-first model.

The exact prose and headings may vary, but these ordering constraints are mandatory. The spec does not require matching heading titles word-for-word if the narrative order and meaning stay intact.

#### The SDK Backbone

This section should introduce the four core authoring concepts together:

1. `DefineLock`
2. `DefineRunOn`
3. `DefineHoldOn`
4. `DefineClaimOn`

The point is not to document every option. The point is to make the mental model obvious before the first code sample.

#### Definition-First Happy Path

The first substantial code example in the README should use a shared definition.

It should show:

1. one `LockDefinition[T]`
2. at least one `RunOn` use case attached to it
3. registry setup
4. client construction
5. one call path such as `Run`

If a second attached use case is shown, it should be minimal and only exist to reinforce that one definition can back multiple surfaces.

For acceptance, the first multi-line SDK code block that demonstrates authoring must include `DefineLock` and at least one attached surface constructor such as `DefineRunOn`, `DefineHoldOn`, or `DefineClaimOn`.

#### When Shorthand Is Enough

This section should explicitly preserve shorthand APIs without giving them narrative priority.

It should say that:

1. `DefineRun`, `DefineHold`, and `DefineClaim` remain valid
2. they are shorthand for one use case owning an implicit private definition
3. they are appropriate when a team does not need shared identity across multiple use cases

This keeps backward-compatible guidance without letting shorthand shape the whole SDK story.

#### Examples By Learning Path

This section should stop being a flat list of example paths.

It should group examples by intent:

1. start here
2. choose a sync or async execution surface
3. shared-definition patterns
4. advanced coordination

The list should emphasize `examples/sdk` as the current public interface and describe `examples/core` as deeper teaching material.

For acceptance, this section must link to `examples/sdk/shared-lock-definition` as the first example presented to the reader.

Non-canonical SDK examples may still be linked, but they must not be presented as alternative starting points. Any additional SDK example links in `README.md` must appear after the canonical learning path and be labeled as additional or scenario-specific follow-up material.

The root `README.md` does not need to mirror the full `examples/README.md` sequence, but it must not contradict it. If the root README presents a shorter example list, that list must still begin with `examples/sdk/shared-lock-definition` and preserve the same first-step priority.

### 2. Examples Index Structure

`examples/README.md` should present the SDK examples as a path through the public model.

Recommended sections:

1. `Start Here`
2. `Choose An Execution Surface`
3. `Shared Definition Patterns`
4. `Advanced Coordination`
5. `About examples/core`

This mapping is normative for the rewrite:

1. `Start Here`
   - `examples/sdk/shared-lock-definition`
2. `Choose An Execution Surface`
   - `examples/sdk/sync-approve-order`
   - `examples/sdk/async-process-order`
3. `Shared Definition Patterns`
   - `examples/sdk/shared-aggregate-split-definitions`
   - `examples/sdk/parent-lock-over-composite`
4. `Advanced Coordination`
   - `examples/sdk/sync-fenced-write`
   - optional references into `examples/core` for strict, composite, lineage, and lower-level preserved scenarios

`examples/sdk/shared-lock-definition` should be named as the canonical first example for the SDK path.

For acceptance, `examples/sdk/shared-lock-definition/README.md` must explicitly teach all of the following minimum concepts:

1. a lock definition is created first
2. at least one execution surface is attached to that definition
3. this is the primary `v1.3.0` SDK authoring model

The rationale for this canonical ordering is part of the spec:

1. `shared-lock-definition` is the smallest focused example that teaches the `v1.3.0` backbone directly
2. `sync-approve-order` and `async-process-order` then show how execution surfaces apply that backbone in familiar flows
3. `shared-aggregate-split-definitions` and `parent-lock-over-composite` extend the backbone into multi-definition modeling choices
4. `sync-fenced-write` is included as the first advanced coordination example because it shows that stricter execution still sits on top of the same SDK authoring model

For acceptance, `examples/core` must not appear before the `examples/sdk` learning path sections, and the `About examples/core` section must explicitly state that new SDK readers should start in `examples/sdk` first.

If `examples/core` is linked from `Advanced Coordination`, those links must appear only after at least one SDK example is listed in that section and must be introduced as optional deeper follow-up material.

Non-canonical SDK examples may still be linked in `examples/README.md`, but they must appear only after the normative learning path and be labeled as additional or scenario-specific follow-up material.

### 3. Canonical Example README Rules

The SDK example READMEs that anchor the learning path should follow a common template shape.

Required fields or sections:

1. one-sentence scenario summary
2. `Backbone concept` or an equivalent clearly labeled short note naming the definition-first lesson
3. `What this example defines`
4. `Why this shape matters`
5. `How to run`

These READMEs do not need to be long. They need to be consistent.

Equivalent headings are allowed, but the content must be discoverable as distinct sections or clearly separated paragraphs that map to the required section meanings.

For acceptance, each required canonical SDK example README must explicitly do all of the following in its explanatory text:

1. mention `lock definition` or `shared definition`
2. mention the attached execution surface directly, either with the phrase `execution surface` or by naming the constructor used by the example
3. avoid presenting shorthand as the primary model unless the README explicitly says it is a convenience path relative to the backbone

The required sections must communicate the following minimum content:

1. `Backbone concept` explains what part of the `v1.3.0` definition-first model this example teaches
2. `What this example defines` names the lock definition and attached surface or surfaces used by the example
3. `Why this shape matters` explains why this example belongs in its position in the canonical learning path

## Vocabulary Rules

The docs should use one consistent vocabulary set across README and example guides:

1. `lock definition`
2. `execution surface`
3. `shared definition`
4. `shorthand`
5. `SDK path`
6. `preserved lower-level examples`

The docs should avoid mixing these with older, less precise framings such as implying that a use case name itself is the primary lock model.

## Examples Positioning Model

The repository currently holds both `examples/sdk` and `examples/core`.

The docs should make this distinction explicit:

1. `examples/sdk` is the public SDK learning path and should win by default for new readers
2. `examples/core` preserves lower-level scenario material and deeper teaching flows
3. duplicated scenarios across both trees are acceptable when the SDK tree mirrors the public interface and the core tree preserves the source scenario framing

The documentation should reduce competition between these trees. `examples/sdk` is where new users start. `examples/core` is where advanced readers go deeper.

## README Status Section

The current root README still says `lockman v1.0.0 is released.`

That line is now too narrow for the intended teaching role of the document. The updated README should avoid sending the reader backward conceptually.

Recommended adjustment:

1. keep the root SDK path framed as the stable public entry point
2. remove or rewrite the current `v1.0.0` release sentence so the status section does not anchor the README to the original release narrative

This can be solved either by revising the wording or removing the version-specific sentence if it no longer helps readers.

## Prominence Rules

To keep `v1.3.0` as the docs backbone, the rewrite should follow these prominence rules:

1. `examples/sdk/shared-lock-definition` is the only example that may be labeled as the starting point or canonical first example
2. shorthand APIs may appear in a dedicated section, but that section must not contain the first substantial authoring code sample and must not contain more multi-line Go code blocks than the definition-first happy-path section
3. `examples/core` may be referenced for deeper study, but it must never be introduced as an equal alternative to the SDK learning path in either `README.md` or `examples/README.md`

## Non-Goals For The Rewrite

The docs rewrite should not overreach into a complete documentation refactor.

It should not:

1. invent new top-level docs unless the rewrite cannot be expressed in the existing README and examples structure
2. duplicate full API reference material that already belongs in dedicated docs
3. teach low-level internals before the public SDK model is clear
4. reframe every advanced topic around shared definitions if that makes specialized docs worse

## Acceptance Criteria

This design succeeds when all of the following are true:

1. `README.md` contains `The SDK Backbone` before its first substantial authoring example
2. the first substantial authoring code path in `README.md` uses `DefineLock` plus an attached execution surface
3. `DefineRun`, `DefineHold`, and `DefineClaim` remain documented, but only as shorthand
4. `examples/README.md` directs new readers first to `examples/sdk/shared-lock-definition` in a `Start Here` section
5. `examples/README.md` places `examples/sdk` learning sections before any explanation of `examples/core`
6. `examples/README.md` explicitly tells new SDK readers to start with `examples/sdk`, and frames `examples/core` as preserved deeper material
7. each required canonical SDK example README contains the required section set and uses aligned vocabulary around lock definitions and execution surfaces
8. the root README status section no longer contains a version-specific sentence centered on `v1.0.0`
9. `README.md` explicitly states that the documentation backbone follows the `v1.3.0` definition-first model
10. `examples/README.md` explicitly states that the SDK examples follow the `v1.3.0` definition-first model
11. no code or API behavior changes are required for the docs rewrite to be considered complete
12. neither `README.md` nor `examples/README.md` presents any non-canonical example as a competing starting point
13. neither `README.md` nor `examples/README.md` presents `examples/core` as an equal alternative to `examples/sdk`

## Verification

Verification should stay proportional to the change.

Required checks:

1. read-through consistency check across all required files listed in `Required Files`
2. verify that `README.md` places `The SDK Backbone` before the first substantial authoring code block and that this first block includes `DefineLock`
3. verify that `examples/README.md` starts the learning path with `examples/sdk/shared-lock-definition` and introduces `examples/core` only afterward
4. verify that each required canonical SDK example README contains the mandatory section set and explicit definition-first wording
5. verify that any non-canonical example links are presented only as follow-up material and not as alternative starting points
6. if any code snippets or runnable example instructions are changed, run:

```bash
go test -tags lockman_examples ./examples/... -run '^$'
```

If root README snippets are changed, compile checks should be considered part of completion.

## Implementation Notes

The implementation should prefer a minimal documentation-only change set:

1. update structure and wording before adding new docs
2. only touch example READMEs that sit on the canonical SDK learning path
3. preserve existing links to advanced docs where they still help the reader descend into detail

If an existing canonical example's code does not fully embody the intended story, the rewrite should correct the README framing without changing code behavior. The docs rewrite does not need to reinterpret the example beyond what its current code actually demonstrates.

If a required canonical example turns out to be a poor fit for the intended learning slot, the implementation should not silently swap it. That should be surfaced as a design mismatch against this spec.

The documentation should explain the `v1.3.0` backbone clearly enough that future examples and quickstarts naturally build on the same model.
