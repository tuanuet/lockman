# Bulk Import Shard Worker Example

This example shows the default boundary for bulk import: one worker owns one shard, and the lock follows that shard boundary.

## What It Teaches

- bulk import usually needs one ownership boundary per shard or partition
- `workers` is the right package because the flow is message-driven and async
- smaller batch-level locks only make sense when batches are independently safe and replayable

## Scenario

Assume the import job is split into shards and this worker is responsible for shard `07`. The business question is not "can I lock one smaller chunk?" but "what is the smallest boundary that still gives safe ownership for all work this worker may perform?" In this teaching case, the answer is the shard.

## Run

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/phase2-bulk-import-shard-worker
```

## Output To Notice

- `shard lock: import-shard:07`
- `package: workers`
- `teaching point: shard ownership is the default boundary for bulk import`
- `contrast: smaller batch locks only work when batches are independently safe and replayable`

## Related Guide

See [`docs/lock-scenarios-and-best-practices.md`](/Users/mrt/workspaces/boilerplate/lockman/docs/lock-scenarios-and-best-practices.md) for the shard or partition ownership scenarios.
