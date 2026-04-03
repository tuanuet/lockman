# Sync Fenced-Write Example

This workspace mirror tracks the public SDK interface. The root `main.go` is gated behind the `lockman_examples` build tag so default root verification stays clean.

## Backbone concept

Strict fenced execution is an advanced coordination mode layered on top of the same SDK authoring surface.

## What this example defines

- one strict sync execution surface for `order.strict-write`
- one strict lock boundary bound to `order:<id>`
- fencing tokens that increase across successive writers

This example uses the strict surface because the scenario needs ordered guarded writes, not just mutual exclusion.

## Why this shape matters

This is the advanced follow-up to the normal SDK backbone.

It shows that stricter execution changes the coordination semantics, but the public SDK path still starts from a typed lock boundary and execution surface.

This specialized strict surface is outside the scope of the root-SDK shorthand deprecation pass, so the example focuses on fencing semantics rather than on the root `Define*On` constructors.

## How to run

Run the SDK workspace mirror from the workspace root:

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/sdk/sync-fenced-write
```

Published adapter runnable path:

```bash
cd backend/redis
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/sync-fenced-write
```
