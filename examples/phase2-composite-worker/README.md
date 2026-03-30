# Phase 2 Composite Worker Example

Archived note: the runnable Go package was removed from the root module during adapter-module extraction. This README remains as historical guidance only and is not part of released-root verification.

This example is the async counterpart to the sync composite example. It combines composite locking with worker-style idempotent claim execution.

## What It Shows

- `workers.ExecuteCompositeClaimed`
- Redis-backed composite lease acquisition
- Redis-backed idempotency completion for one async composite job
- Canonical ordering of composite member resource keys in worker callbacks

## Status

This root path is archived. Keep using it only as historical documentation while the adapter-module refactor is in flight.

## Flow

1. Register two async member definitions: `AccountMember` and `LedgerMember`.
2. Register one async composite definition named `TransferComposite`.
3. Execute a composite claim request with one idempotency key.
4. Inside the callback, inspect the ordered list of composite resource keys.
5. After the callback returns, inspect the persisted idempotency record.

## Output To Notice

- `composite callback: account:acct-123,ledger:ledger-456`
  This proves the worker acquired the whole composite and entered the callback with canonical ordering.
- `composite idempotency after ack: completed`
  This proves the composite worker path persisted terminal idempotency state after success.
- `shutdown: ok`
  This proves the worker manager shut down cleanly after the claim completed.

## When To Use This Example

Use this when one queue message must atomically claim multiple resources before the handler runs.
