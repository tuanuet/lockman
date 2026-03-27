# Phase 3a Strict Runtime Example

This example demonstrates strict single-resource runtime execution in Phase 3a using the in-memory driver.

## What It Shows

- Strict sync definition registration
- Fencing token visibility inside `runtime.ExecuteExclusive` callbacks
- Sequential reacquire issuing a larger fencing token on the same lock boundary
- Current Phase 3a limit: runtime strict execution still fits one TTL window (no renewal loop)

## Run

```bash
go run ./examples/phase3a-strict-runtime
```

## Output To Notice

- `strict runtime lock: order:123`
  The strict callback runs on the expected resource key.
- `fencing token first: 1`
  The first strict acquire on a fresh boundary starts fencing at `1`.
- `fencing token second: 2`
  A sequential reacquire on the same boundary receives a larger token.
- `teaching point: strict runtime exposes fencing tokens but still relies on one ttl window in phase3a`
  Phase 3a exposes tokens, but guarded-write persistence coordination is not complete yet.
