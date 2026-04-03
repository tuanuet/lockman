# Shared Lock Definition Example

This is the canonical first example for the `v1.3.0` SDK path.

## Backbone concept

Create one shared lock definition first, then attach execution surfaces to that definition.

This is the primary `v1.3.0` authoring model for the SDK.

## What this example defines

- one lock definition: `contractDef`
- one sync execution surface: `DefineRunOn("contract.import", contractDef)`
- one hold execution surface: `DefineHoldOn("contract.manual_hold", contractDef)`

Both public use cases share the same lock definition and resolve the same resource key for `contract:42`.

## Why this shape matters

This is the smallest example that shows the SDK backbone directly:

- the lock definition owns the shared identity
- the execution surfaces attach behavior to that same boundary
- registry wiring and execution stay on the root SDK path

## How to run

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/sdk/shared-lock-definition
```
