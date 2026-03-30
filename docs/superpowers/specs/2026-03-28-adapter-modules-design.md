# Lockman Adapter Modules Design

## Goal

Move concrete infrastructure adapters out of `lockkit` and make them separate Go modules, while keeping the root `lockman` SDK clean and stable.

The intended end state is:

- core contracts live in stable top-level packages
- `lockkit` becomes an internal engine area rather than a place where users import concrete adapters
- Redis and Postgres adapters can evolve behind independent module boundaries now, with independent external versioning later

## Scope

### In Scope

- move Redis lock backend implementation out of `lockkit/drivers/redis`
- move Redis idempotency implementation out of `lockkit/idempotency/redis`
- move Postgres guarded-write helper out of `lockkit/guard/postgres`
- define stable top-level contracts for backend, idempotency, and guard
- convert each adapter area into its own Go module inside the monorepo
- migrate first-contact docs and supported examples to the new adapter module paths
- keep `lockman.New(...)`, `WithBackend(...)`, and `WithIdempotency(...)` working through stable contracts

### Out Of Scope

- changing lock semantics
- changing registry semantics
- redesigning the `Client.Run(...)` and `Client.Claim(...)` happy path
- splitting adapters into separate repositories in this phase
- final public canonical module path rewrite

## Problem Statement

The current codebase still mixes three concerns:

1. public SDK surface
2. internal lock engine
3. concrete infrastructure adapters

That creates two problems.

First, `lockkit` is supposed to become internal engine territory, but it still contains concrete packages that users can import directly:

- `lockkit/drivers/redis`
- `lockkit/idempotency/redis`
- `lockkit/guard/postgres`

Second, historical and engine-level examples still import those low-level adapter packages directly, which keeps the internal engine boundary blurry even though the new first-contact SDK path already prefers `lockman/backend/redis` and `lockman/idempotency/redis`.

Third, the root module currently carries third-party dependencies that belong to adapters rather than to the SDK core:

- `go-redis`
- `pgx`

This makes the repository look more like one large implementation bundle than a clean SDK with separable adapters.

## Design Constraints

The refactor must preserve these product rules:

- centralized registry remains mandatory
- root `lockman` remains the default SDK path
- advanced packages remain explicit
- breaking changes are acceptable
- callsite UX should not get worse for the happy path
- internal engine packages should not become the long-term public extension story

## Architecture Options

### Option A: Move Concrete Folders Into Nested Modules, Keep Contracts In `lockkit`

Example shape:

- `adapters/redis` imports `lockman/lockkit/drivers`
- `adapters/idempotency/redis` imports `lockman/lockkit/idempotency`
- `adapters/guard/postgres` imports `lockman/guard`

This is the smallest code move.

It is not the recommended option because it preserves `lockkit` as a semi-public contract layer. That conflicts with the goal of making `lockkit` engine-internal.

### Option B: Promote Stable Contracts To Top-Level Packages, Then Move Adapters Into Nested Modules

Example shape:

- `backend` exposes the backend driver contracts
- `idempotency` exposes the store contracts
- `guard` exposes guarded-write context and outcomes
- adapter modules depend on these contracts rather than on `lockkit/...`

This is the recommended option.

It creates a clean separation:

- root-level stable contracts for integrators and adapter authors
- `lockkit` reserved for engine implementation
- concrete adapters isolated into separate modules

This option requires one extra rule to be explicit:

- any shared type that adapter modules need must be promoted out of `lockkit`

### Option C: Split Adapters Into Separate Repositories Immediately

This is the cleanest release boundary, but it adds unnecessary operational cost right now:

- separate repository setup
- multi-repo local development
- cross-repo atomic refactors become harder

This should wait until the adapter module boundaries are proven inside the monorepo.

## Recommended Design

Use Option B.

The repository should become a multi-module monorepo with a contract-first core.

## Public Package Model

### Root Module Responsibilities

The root `lockman` module should contain:

- user-first SDK surface
- stable integration contracts
- internal engine implementation

It should not contain concrete Redis or Postgres adapter implementations.

### Stable Top-Level Contract Packages

Promote the stable contracts out of `lockkit` into top-level packages:

- `backend`
- `idempotency`
- `guard`

These packages become the supported contract layer for:

- `lockman.WithBackend(...)`
- `lockman.WithIdempotency(...)`
- guarded-write integrations that are intentionally outside the engine
- adapter modules

### Shared Boundary Rule

Adapter modules must not depend on arbitrary `lockkit/...` packages.

The refactor should follow this rule:

- if a type is part of the supported adapter authoring surface, promote it to a top-level contract package
- if a type is only meaningful inside the engine, keep it in `lockkit`

The target is to avoid accidental adapter dependencies on:

- `lockkit/definitions`
- `lockkit/errors`
- engine-only runtime or worker internals

This means the migration must evaluate shared seams intentionally instead of moving adapter files first and discovering hidden `lockkit` dependencies later.

This spec intentionally does not allow a temporary whitelist for adapter-module dependencies on `lockkit/...`.

### `lockkit` Responsibilities After Refactor

`lockkit` remains in the root module, but it should be treated as engine implementation detail:

- runtime execution
- workers execution
- registry internals
- engine translation and policy
- definitions and testkit
- internal memory-only helpers used for tests or engine-local execution

Any contracts still needed by the engine should be imported from the new top-level contract packages where appropriate.

## Adapter Module Layout

The monorepo should introduce separate `go.mod` files for the concrete adapters.

### Redis Backend Module

Recommended path:

- `redis/go.mod`
- module path remains unresolved until canonical publishing is decided, but the module boundary should be real now

This module owns:

- current Redis backend implementation
- Redis-specific tests
- Redis-specific third-party dependencies

The root package `lockman/backend/redis` should no longer be a thin wrapper around an internal `lockkit` package. It becomes the adapter module itself.

### Redis Idempotency Module

Recommended path:

- `idempotency/redis/go.mod`

This module owns:

- current Redis idempotency implementation
- Redis idempotency tests
- Redis idempotency dependencies

### Postgres Guard Module

Recommended path:

- `guard/postgres/go.mod`

This module owns:

- current Postgres guarded-write helper
- Postgres helper tests
- Postgres driver dependency

It should depend on the top-level `guard` contract package rather than on `lockkit/guard`.

### Workspace Wiring

The monorepo should use a root-level `go.work` file for local development and CI.

The initial workspace should include:

- `.`
- `./backend/redis`
- `./idempotency/redis`
- `./guard/postgres`

The root `go.mod` should not use local `replace` directives to stitch these adapter modules together.

The intended development model is:

- `go.work` binds the modules together inside the monorepo
- each module keeps its own `go.mod`
- root and adapter CI commands run intentionally against the workspace rather than relying on ad-hoc local replacements

This workspace model is for monorepo development only.

For released-module behavior, the root module should not rely on sibling adapter modules being present through `go.work`.

To keep that boundary clean:

- runnable adapter-dependent examples should move into the relevant adapter modules
- root-module docs may still show adapter usage snippets, but the root module should not keep runnable examples or tests that require sibling adapter modules to exist
- published root-module validation with `GOWORK=off` should still succeed without adapter modules checked out next to it

## Contract Migration

### Backend Contract

Create a top-level `backend` package that contains the current driver contracts and optional capabilities:

- `Driver`
- `StrictDriver`
- `LineageDriver`
- request and record types
- backend-level sentinel errors used across adapters and engine
- any shared lineage metadata enums needed by the backend contract

`lockman.WithBackend(...)` should accept `backend.Driver`.

If any current backend contract types still depend on `lockkit/definitions`, they must either:

- be rewritten to use promoted top-level shared types, or
- be simplified so adapter modules do not need those engine definition types directly

The preferred outcome is that adapter modules do not need `lockkit/definitions` at all.

This is a design requirement, not an implementation-time choice.

The current `definitions.LockKind` seam should be resolved by promoting a backend-scoped shared kind type rather than leaving lineage-capable adapters coupled to `lockkit/definitions`.

The current driver-level sentinel errors should also move into `backend`, so adapter modules and engine code preserve `errors.Is(...)` compatibility without importing `lockkit/errors`.

### Idempotency Contract

Create a new top-level `idempotency` contract package as the stable store contract layer.

The current `lockkit/idempotency` contracts should migrate into that package or be replaced by it so the root SDK no longer depends on `lockkit/idempotency` as the public contract source.

### Guard Contract

Create a top-level `guard` package and move:

- `Context`
- `Outcome`

The new Postgres adapter module should import this package.

The existing `advanced/guard` namespace remains documentation-only for now and should not collide semantically with the new `guard` contract package.

This contract layer is for adapter authors and lower-level integrations in this phase, not for the default `Run(...)` or `Claim(...)` happy path.

The public `lockman.Lease` and `lockman.Claim` types should not expand in this refactor.

That means lease/claim-to-guard mapping should move behind a root-internal bridge built from internal execution contexts rather than being exposed as part of the top-level `guard` contract.

So the top-level `guard` package should expose stable data types only:

- `Context`
- `Outcome`
- any guard-scoped sentinel errors needed by guard adapter modules

## SDK Surface Impact

### Happy Path

The user-first path should stay nearly the same. This refactor is mainly internal-boundary cleanup plus supported adapter module extraction, not a repair of the current first-contact SDK path.

```go
client, err := lockman.New(
    lockman.WithRegistry(reg),
    lockman.WithIdentity(lockman.Identity{OwnerID: "orders-api"}),
    lockman.WithBackend(redis.New(redisClient, "")),
)
```

and

```go
client, err := lockman.New(
    lockman.WithRegistry(reg),
    lockman.WithIdentity(lockman.Identity{OwnerID: "orders-worker"}),
    lockman.WithBackend(redis.New(redisClient, "")),
    lockman.WithIdempotency(redisidempotency.New(redisClient, "")),
)
```

The callsite should not learn about `lockkit` to use supported adapters.

### Low-Level And Historical Paths

Examples and docs that currently import `lockkit/drivers/redis`, `lockkit/idempotency/redis`, or `lockkit/guard/postgres` should be handled intentionally:

- keep supported first-contact examples on the top-level adapter paths
- move runnable adapter-dependent examples into the relevant adapter modules
- either migrate or explicitly archive engine-level historical examples
- stop teaching direct imports from `lockkit` for supported adapter usage

## Dependency Model

The root `go.mod` should drop adapter-only dependencies once the move is complete, unless still needed by tests or examples that remain in the root module.

Target outcome:

- root module should not need `go-redis` for core SDK compilation
- root module should not need `pgx` for core SDK compilation
- adapter modules own their infrastructure dependencies

This does not require external versioning to be solved yet. The immediate goal is real module boundaries inside the monorepo.

## Testing Strategy

The refactor needs verification at three layers.

### Root Module

Verify:

- root SDK compiles against top-level contracts
- existing client tests still pass
- root-module verification passes with `GOWORK=off`
- root module no longer requires runnable adapter-dependent examples to compile

### Adapter Modules

Each adapter module should have its own focused test suite through its own `go.mod`.

### Whole-Repository Validation

The monorepo should still support a top-level verification story that runs all modules intentionally, rather than assuming one `go test ./...` from the root covers nested modules automatically.

This means the plan should add explicit multi-module verification commands.

The verification model should assume `go.work` is present and active.

It should also include an explicit released-root check with `GOWORK=off` so workspace-only success does not hide broken published-module behavior.

## Migration Order

The migration should happen in this order:

1. introduce stable top-level contracts
2. promote the shared types needed by adapter modules out of `lockkit`
3. add `go.work` and wire the nested modules into the monorepo workspace
4. repoint root SDK internals to those contracts
5. move concrete adapter implementations into nested modules
6. move runnable adapter-dependent examples into adapter modules and retarget root docs
7. clean up remaining `lockkit` adapter imports

This order keeps the refactor understandable and avoids mixing contract redesign with concrete adapter moves in one unreadable step.

## Risks

### Package Naming Risk

The new top-level `guard` contract package may be confused with `advanced/guard`.

Mitigation:

- treat `guard` as contract package
- keep `advanced/guard` as advanced docs namespace only
- document the difference explicitly

### Multi-Module Tooling Risk

Nested modules are not covered automatically by root-level commands.

Mitigation:

- add explicit verification commands
- update docs for contributor workflows

### Historical Example Drift

The repository still contains many phase-oriented examples that import low-level packages directly.

Mitigation:

- classify them as either supported examples to migrate or archived historical examples
- do not leave them half-migrated

### Incomplete Contract Extraction Risk

Some current adapter seams depend on `lockkit/definitions`, `lockkit/errors`, and internal execution contexts.

Mitigation:

- identify those seams explicitly before moving files
- promote the shared types that adapter modules actually need
- reject any implementation that leaves adapter modules importing `lockkit/...`

### Workspace-Only Success Risk

`go.work` can make local CI green while the published root module is still broken when consumed alone.

Mitigation:

- keep runnable adapter-dependent examples out of the root module
- run an explicit `GOWORK=off` root-module verification step
- treat any root-module dependency on sibling adapter modules as a design failure for this phase

### Memory Adapter Ambiguity

The in-memory idempotency store currently lives under `lockkit/idempotency`.

Mitigation:

- move `NewMemoryStore` into the new top-level `idempotency` package
- treat it as supported in-memory utility rather than as an engine-local adapter

## Success Criteria

This refactor is successful when:

- no supported adapter lives under `lockkit`
- Redis and Postgres adapters are their own Go modules
- root SDK depends on stable top-level contracts rather than adapter internals
- first-contact docs and supported examples no longer teach `lockkit` adapter imports
- `lockkit` reads as engine implementation rather than as the home of supported concrete adapters
- adapter modules do not import `lockkit/...`
- the repository builds and tests through an explicit `go.work`-based multi-module workflow
- the root module also verifies successfully with `GOWORK=off`
- the in-memory idempotency utility no longer lives under `lockkit`
