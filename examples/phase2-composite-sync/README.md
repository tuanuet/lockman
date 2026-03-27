# Phase 2 Composite Sync Example

This example shows sync composite execution without Redis by using the in-memory driver.

## What It Shows

- `runtime.ExecuteCompositeExclusive`
- Canonical ordering across multiple member locks
- All-or-nothing acquisition for a composite request
- A minimal composite flow that is deterministic and easy to read

## Prerequisites

- No external services

## Run

```bash
go run ./examples/phase2-composite-sync
```

## Flow

1. Register two parent lock definitions: `AccountMember` and `LedgerMember`.
2. Register one composite definition named `TransferComposite`.
3. Execute the composite with one account key and one ledger key.
4. Observe that callback output uses canonical member ordering, not input order.

## Output To Notice

- `composite acquired: account:acct-123,ledger:ledger-456`
  This proves the composite acquired both members and normalized their order.
- `canonical order: ok`
  This proves the example validated the expected global order.
- `shutdown: ok`
  This proves the runtime manager drained cleanly after execution.

## When To Use This Example

Use this when you want to model one sync operation that must hold multiple resources together, such as a transfer or coordinated update.
