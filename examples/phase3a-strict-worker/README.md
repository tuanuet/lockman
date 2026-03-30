# Phase 3a Strict Worker Example

Archived note: the runnable Go package was removed from the root module during adapter-module extraction. This README remains as historical guidance only and is not part of released-root verification.

This example demonstrates strict async worker execution in Phase 3a using Redis leases and Redis idempotency state.

## What It Shows

- Strict async definition registration with required idempotency
- Fencing token visibility inside `workers.ExecuteClaimed` callbacks
- Terminal idempotency state after successful callback completion
- Current Phase 3a limit: guarded persistence writes remain Phase 3b work

## Status

This root path is archived. Keep using it only as historical documentation while the adapter-module refactor is in flight.

## Output To Notice

- `strict worker claim: order:123`
  The strict async callback ran for the expected resource key.
- `fencing token: 1`
  Strict worker claims expose the fencing token in callback context.
- `idempotency after ack: completed`
  Async strict flows still persist terminal idempotency outcomes.
- `teaching point: strict worker exposes fencing tokens; guarded writes still arrive in phase3b`
  The lock token is available in Phase 3a; persistence-side guarded write integration is deferred.
