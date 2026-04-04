# Manual Hold Example

This workspace mirror demonstrates the definition-first SDK hold path.

## What it shows

- One lock definition bound to `order:<id>`
- One hold surface attached to that definition via `DefineHoldOn(...)`
- Acquiring a hold and later forfeiting it via `Client.Forfeit(...)`

Use `Hold` when a user or process needs to retain an explicit lock across steps, not just protect a single request/response callback.

## How to run

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/sdk/manual-hold
```
