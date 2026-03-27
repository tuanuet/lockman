# Phase 3a Strict Worker Example

This example demonstrates strict async worker execution in Phase 3a using Redis leases and Redis idempotency state.

## What It Shows

- Strict async definition registration with required idempotency
- Fencing token visibility inside `workers.ExecuteClaimed` callbacks
- Terminal idempotency state after successful callback completion
- Current Phase 3a limit: guarded persistence writes remain Phase 3b work

## Prerequisites

- Redis running locally
- `LOCKMAN_REDIS_URL` pointing to that Redis instance

## Run

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/phase3a-strict-worker
```

## Output To Notice

- `strict worker claim: order:123`
  The strict async callback ran for the expected resource key.
- `fencing token: 1`
  Strict worker claims expose the fencing token in callback context.
- `idempotency after ack: completed`
  Async strict flows still persist terminal idempotency outcomes.
- `teaching point: strict worker exposes fencing tokens; guarded writes still arrive in phase3b`
  The lock token is available in Phase 3a; persistence-side guarded write integration is deferred.
