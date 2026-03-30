# Async Bulk-Import Shard Example

This example source is kept in the root workspace. Its `main.go` is gated behind the `lockman_examples` build tag so default root verification does not depend on sibling adapter modules.

This example shows the default boundary for bulk import: one worker owns one shard, and the lock follows that shard boundary.

## What It Teaches

- bulk import usually needs one ownership boundary per shard or partition
- `workers` is the right package because the flow is message-driven and async
- smaller batch-level locks only make sense when batches are independently safe and replayable

## Scenario

Assume the import job is split into shards and this worker is responsible for shard `07`. The business question is not "can I lock one smaller chunk?" but "what is the smallest boundary that still gives safe ownership for all work this worker may perform?" In this teaching case, the answer is the shard.

## Status

- This remains a runnable workspace example.
- It intentionally uses the lower-level `registry` and `workers` APIs because it demonstrates an advanced worker boundary choice.
- If you want the default user-facing API first, start with [`docs/quickstart-async.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/quickstart-async.md).

## Run

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/core/async-bulk-import-shard
```

## Output To Notice

- `shard lock: import-shard:07`
- `package: workers`
- `teaching point: shard ownership is the default boundary for bulk import`
- `contrast: smaller batch locks only work when batches are independently safe and replayable`

## Related Guide

See [`docs/lock-scenarios-and-best-practices.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-scenarios-and-best-practices.md) for the shard or partition ownership scenarios.
