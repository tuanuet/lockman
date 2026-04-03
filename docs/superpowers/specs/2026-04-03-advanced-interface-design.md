# Advanced Interface Design

## Status

Approved for specification drafting

## Goal

Align the `advanced/` public package surface with the repository's definition-first SDK model.

After this design, advanced authoring should keep one consistent grammar:

1. define a lock boundary first
2. attach a run surface onto that definition
3. use advanced packages only when the boundary shape or attach semantics are advanced

The design should remove the current mismatch where the root SDK teaches `DefineLock + DefineRunOn`, while `advanced/strict` and `advanced/composite` pull users back toward shorthand-style `DefineRun` wrappers.

This design assumes a breaking change is acceptable for the `advanced/` surface if that is what the cleaner interface requires.

## Problem Statement

The root SDK story is now definition-first:

1. `DefineLock` creates the lock boundary
2. `DefineRunOn`, `DefineHoldOn`, and `DefineClaimOn` attach execution surfaces
3. advanced behavior should feel like a variation on that same model, not a separate authoring style

But the current `advanced/` packages do not follow that shape.

Current state:

1. `advanced/strict.DefineRun(...)` wraps `lockman.DefineRun(..., lockman.Strict())`
2. `advanced/composite.DefineRun(...)` wraps `lockman.DefineRun(...)` with `lockman.Composite(...)`
3. `advanced/composite.DefineMember(...)` introduces a `Member` concept that is semantically closer to a reusable lock definition than a new primitive

This creates three interface problems:

1. advanced packages reintroduce the shorthand constructor shape that the root SDK is actively de-emphasizing
2. `DefineRunOn` no longer has one stable meaning across the public API surface
3. `DefineMember` names the concept incorrectly, because each member is effectively its own lock definition

## Decision

The advanced interface should be normalized around definitions.

Normative decisions:

1. `DefineRunOn` should always mean attaching a run surface to an existing definition
2. `advanced/strict` should expose `DefineRunOn`, not promote `DefineRun`
3. composite child parts should be authored as reusable `lockman.LockDefinition[T]` values
4. `advanced/composite` should compose definitions instead of inventing a separate `Member` authoring primitive

The intended public grammar becomes:

```go
// normal
orderDef := lockman.DefineLock("order", ...)
approve := lockman.DefineRunOn("order.approve", orderDef)

// strict
orderDef := lockman.DefineLock("order", ...)
approve := strict.DefineRunOn("order.approve", orderDef)

// composite
accountDef := lockman.DefineLock("account", ...)
ledgerDef := lockman.DefineLock("ledger", ...)

transferDef := composite.DefineLock("transfer", accountDef, ledgerDef)
transfer := lockman.DefineRunOn("transfer.run", transferDef)
```

## Scope

In scope:

1. define the target API shape for `advanced/strict`
2. define the target API shape for `advanced/composite`
3. define removal direction for `strict.DefineRun`, `composite.DefineRun`, and `composite.DefineMember`
4. define the migration story from current advanced wrappers to the new shape
5. clarify the intended meaning of composite child definitions

Out of scope:

1. implementation details of engine/runtime support
2. whether this lands in one PR or multiple PRs
3. redesigning `advanced/lineage` or `advanced/guard` beyond alignment notes
4. root-SDK deprecation policy already covered by shorthand constructor specs

## Design Principles

### One Public Grammar

The public authoring model should not split into:

1. root path: `DefineLock -> DefineRunOn`
2. advanced path: `DefineRun(binding, ...)`

That split forces users to keep two mental models for one SDK. Advanced features should refine the same grammar, not replace it.

### Honest Names

If a value has a stable name and binding and can be reused in multiple composite boundaries, it is a lock definition in all meaningful API senses.

Calling that value a `Member` obscures what the user is actually authoring.

### Advanced Packages Own Only The Advanced Part

Advanced packages should exist only where they add domain meaning:

1. `strict` changes how a run surface behaves
2. `composite` changes what kind of boundary is being defined

They should not re-wrap the whole root authoring pipeline when only one step is advanced.

## Strict Package Design

### Target API

`advanced/strict` should expose:

```go
func DefineRunOn[T any](name string, def lockman.LockDefinition[T], opts ...lockman.UseCaseOption) lockman.RunUseCase[T]
```

Expected behavior:

1. preserve the root `DefineRunOn` meaning of attaching a run surface to an existing definition
2. append strict run behavior during that attach step
3. preserve any caller-provided use case options
4. guarantee strict behavior regardless of caller-provided option order

Option precedence rule:

1. `strict.DefineRunOn(...)` must always produce a strict run use case
2. if the caller also passes `lockman.Strict()`, the result is still just strict behavior, not a second distinct mode
3. no caller-provided option may disable strictness on this API
4. implementation may normalize duplicate strict options internally, but public behavior must be unambiguously strict

Illustrative implementation shape:

```go
func DefineRunOn[T any](name string, def lockman.LockDefinition[T], opts ...lockman.UseCaseOption) lockman.RunUseCase[T] {
	return lockman.DefineRunOn(name, def, append(opts, lockman.Strict())...)
}
```

### Deprecated Compatibility Surface

`strict.DefineRun(...)` should be removed as part of the advanced-surface cleanup.

If an implementation phase temporarily keeps it during an intermediate branch, that state should not be documented as the target public design.

## Composite Package Design

### Current Misfit

`DefineMember(...)` is currently presented as if a composite run is built from a list of special members.

But the actual semantics are closer to this:

1. each child has its own name
2. each child has its own binding
3. each child identifies one lock identity inside the larger operation

That is definition-shaped, not member-shaped.

### Target API

The composite package should expose a definition constructor, not a special run constructor.

Target shape:

```go
func DefineLock[T any](name string, defs ...lockman.LockDefinition[T]) lockman.LockDefinition[T]
```

This is the preferred end state because it preserves one shared public abstraction for boundaries: `lockman.LockDefinition[T]`.

Homogeneous input type rule:

1. all child definitions in one composite definition must share the same input type `T`
2. this design does not support heterogeneous child input types in one composite boundary
3. if different resources need different input extraction logic, callers should normalize them into one shared operation input struct before defining child locks

The intended usage is:

```go
accountDef := lockman.DefineLock("account", ...)
ledgerDef := lockman.DefineLock("ledger", ...)

transferDef := composite.DefineLock("transfer", accountDef, ledgerDef)
transfer := lockman.DefineRunOn("transfer.run", transferDef)
```

### Child Definitions Are Reusable

Composite child parts should be real reusable `lockman.LockDefinition[T]` values.

This is normative.

The design explicitly rejects a model where child parts are only internal anonymous composite-only members.

Reasons:

1. reusable definitions are easier to name, test, and reason about
2. the API becomes more honest because it uses one existing concept instead of inventing a second one
3. a child definition can be reused across multiple composite definitions when that matches the business model
4. the SDK vocabulary stays compact and consistent

### Fallback If Core Types Cannot Represent This Directly

If the existing `lockman.LockDefinition[T]` type cannot represent composite boundaries without unacceptable internal distortion, the allowed fallback is:

```go
type Definition[T any] struct { ... }

func DefineLock[T any](name string, defs ...lockman.LockDefinition[T]) Definition[T]
func AttachRun[T any](name string, def Definition[T], opts ...lockman.UseCaseOption) lockman.RunUseCase[T]
```

This fallback is acceptable only if the preferred shared-definition shape is not technically viable.

The fallback deliberately must not reuse the name `DefineRunOn`, because that name is reserved for APIs that accept the shared root definition abstraction directly.

Even in the fallback model, the following rules remain mandatory:

1. `DefineMember` should still disappear as the primary concept
2. child parts should still be authored as root `lockman.LockDefinition[T]`
3. the public docs should still teach composite as a definition-first flow
4. the preferred implementation target remains `lockman.DefineRunOn("...", composite.DefineLock(...))`

## Breaking-Change Direction

The following advanced APIs should be removed from the target public surface:

1. `advanced/strict.DefineRun`
2. `advanced/composite.DefineRun`
3. `advanced/composite.DefineRunWithOptions`
4. `advanced/composite.DefineMember`

The user has explicitly approved breaking change latitude for this cleanup. The design should therefore optimize for interface clarity rather than temporary compatibility shims.

Short-lived transitional wrappers may exist on a private branch while the implementation is in progress, but they are not part of the desired steady-state API.

Required removal policy:

1. the old advanced exported constructors should be removed in the same user-facing change that introduces the replacement API
2. docs and examples should be updated in that same change so the repository does not temporarily teach removed names as current APIs
3. compile-time breakage for the removed names is expected immediately after the change lands
4. no temporary exported compatibility period is part of the target release shape

## Migration Story

### Strict

Before:

```go
approve := strict.DefineRun("order.approve", binding)
```

After:

```go
orderDef := lockman.DefineLock("order", binding)
approve := strict.DefineRunOn("order.approve", orderDef)
```

Old strict constructors are removed, not deprecated.

### Composite

Before:

```go
transfer := composite.DefineRun(
	"transfer.run",
	composite.DefineMember("account", ...),
	composite.DefineMember("ledger", ...),
)
```

After:

```go
accountDef := lockman.DefineLock("account", ...)
ledgerDef := lockman.DefineLock("ledger", ...)

transferDef := composite.DefineLock("transfer", accountDef, ledgerDef)
transfer := lockman.DefineRunOn("transfer.run", transferDef)
```

Migration guidance should emphasize that the new child definitions are allowed to stay private inside one package. Reusability is a capability, not a requirement for valid use.

Because breaking change is allowed here, migration docs do not need to preserve old advanced constructor names as supported compatibility aliases.

Old composite constructors are removed, not deprecated.

## Documentation Rules

After implementation:

1. `docs/advanced/strict.md` should teach `strict.DefineRunOn`
2. `docs/advanced/composite.md` should teach composite child definitions as `lockman.DefineLock(...)`
3. package docs in `advanced/strict` and `advanced/composite` should describe these packages as definition-first advanced paths
4. examples should stop introducing `DefineMember` as the main composite authoring concept

`advanced/lineage` and `advanced/guard` do not need new APIs in this pass, but their package docs should not contradict the normalized definition-first framing if they are touched later.

## Acceptance Criteria

This design is satisfied when all of the following are true:

1. `advanced/strict` exports `DefineRunOn(name, def, ...)` and does not export `DefineRun`
2. `strict.DefineRunOn(...)` always produces strict behavior regardless of caller option ordering or duplicate `Strict()` options
3. the preferred composite API accepts child `lockman.LockDefinition[T]` values with one shared input type `T`
4. `advanced/composite` does not export `DefineMember`, `DefineRun`, or `DefineRunWithOptions`
5. if the preferred composite shape is technically viable, composite runs are attached with root `lockman.DefineRunOn(...)`
6. if the preferred composite shape is not technically viable, the fallback API must avoid reusing the name `DefineRunOn`
7. docs and examples in the same implementation change teach only the new advanced APIs
8. `DefineRunOn` keeps one stable public meaning wherever that name appears: attach a run surface to a definition supported by that package's definition abstraction

## Open Technical Question

One implementation question remains intentionally open for the coding phase:

Can `composite.DefineLock(...)` return a true `lockman.LockDefinition[T]` without compromising the internal definition model?

The implementation phase should answer that question. Until proven otherwise, the preferred design target is yes.
