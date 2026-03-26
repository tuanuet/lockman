# Lock Management Platform SDK for Go: Phase 2 Examples Design

## Status

Draft

## Relationship to Existing Specs

This document defines the example coverage to add on top of the Phase 2 platform implementation described in:

- [2026-03-26-lock-management-platform-design.md](/Users/mrt/workspaces/boilerplate/lockman/docs/superpowers/specs/2026-03-26-lock-management-platform-design.md)
- [2026-03-26-lock-management-platform-phase-2-design.md](/Users/mrt/workspaces/boilerplate/lockman/docs/superpowers/specs/2026-03-26-lock-management-platform-phase-2-design.md)

It does not introduce new runtime behavior. It only defines how the repository should demonstrate the already-approved Phase 2 capabilities through focused runnable examples.

## Goal

Add small Phase 2 examples that each demonstrate one user-facing capability clearly, with deterministic output and matching tests.

The examples should make it easy to answer:

- how do I run a basic worker claim with Redis and idempotency?
- how do I run a standard-mode composite lock synchronously?
- how do I run a standard-mode composite claim asynchronously?
- what does Phase 2 reject-first parent-child overlap behavior look like?

## Design Principles

- Each example demonstrates exactly one primary Phase 2 concept.
- Output must be short, stable, and suitable for string-based tests.
- Redis-backed examples must skip tests when `LOCKMAN_REDIS_URL` is unset.
- Memory-driver examples should not depend on timing-sensitive behavior.
- Examples should explain Phase 2 behavior, not reproduce every internal edge case already covered by package tests.

## Example Set

### `examples/phase2-basic`

Purpose:

- demonstrate single-resource worker execution
- demonstrate Redis-backed presence inspection while the claim is held
- demonstrate idempotency completion after successful execution
- demonstrate duplicate suppression on reprocessing

Implementation direction:

- keep the existing scope and flow
- use `workers.NewManager`
- use `lockkit/drivers/redis`
- use `lockkit/idempotency/redis`

Expected output:

```text
execute: callback running for order:123
presence while held: held
idempotency after ack: completed
duplicate outcome: ignored
shutdown: ok
```

### `examples/phase2-composite-sync`

Purpose:

- demonstrate `ExecuteCompositeExclusive`
- demonstrate canonical member ordering
- demonstrate that callback code receives composite `ResourceKeys`

Implementation direction:

- use `runtime.NewManager`
- use `testkit.NewMemoryDriver`
- register a two-member standard-mode composite such as account plus ledger

Expected output:

```text
composite acquired: account:acct-123,ledger:ledger-456
canonical order: ok
shutdown: ok
```

### `examples/phase2-composite-worker`

Purpose:

- demonstrate `ExecuteCompositeClaimed`
- demonstrate composite worker callback resource ordering
- demonstrate composite idempotency completion after successful execution

Implementation direction:

- use `workers.NewManager`
- use `lockkit/drivers/redis`
- use `lockkit/idempotency/redis`
- register a two-member async composite

Expected output:

```text
composite callback: account:acct-123,ledger:ledger-456
composite idempotency after ack: completed
shutdown: ok
```

### `examples/phase2-overlap-reject`

Purpose:

- demonstrate the Phase 2 reject-first parent-child overlap policy
- show that overlap is rejected before callback execution

Implementation direction:

- use `runtime.NewManager`
- use `testkit.NewMemoryDriver`
- register a parent definition and child definition with `ParentRef`
- set child `OverlapPolicy` to `reject`
- register a composite that attempts to include overlapping parent and child resources in the same tree

Expected output:

```text
overlap outcome: rejected
shutdown: ok
```

## Test Strategy

- Each example gets its own `main_test.go`.
- Tests assert the full user-visible output contract for that example.
- Redis-backed example tests skip when `LOCKMAN_REDIS_URL` is unset.
- Memory-driver examples run as ordinary unit-style example tests.

The examples do not need to cover:

- renewal failure
- partial rollback internals
- all worker outcome mappings
- registry validation edge cases

Those behaviors already belong in package-level tests.

## Planned File Changes

Add:

- `examples/phase2-composite-sync/main.go`
- `examples/phase2-composite-sync/main_test.go`
- `examples/phase2-composite-worker/main.go`
- `examples/phase2-composite-worker/main_test.go`
- `examples/phase2-overlap-reject/main.go`
- `examples/phase2-overlap-reject/main_test.go`

Modify:

- `README.md`

Keep existing scope:

- `examples/phase2-basic/main.go`
- `examples/phase2-basic/main_test.go`

## Verification

At implementation time, verify with:

```bash
go test ./examples/...
go test ./...
```

When Redis is available, Phase 2 Redis examples should also be runnable directly:

```bash
go run ./examples/phase2-basic
go run ./examples/phase2-composite-worker
```

Memory-backed Phase 2 examples should be runnable without external services:

```bash
go run ./examples/phase2-composite-sync
go run ./examples/phase2-overlap-reject
```

## Non-Goals

- No new public API design
- No new Phase 2 runtime features
- No example for every internal test scenario
- No strict-mode example coverage
- No queue-product-specific examples
