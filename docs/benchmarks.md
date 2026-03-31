# Benchmarks

These benchmarks give application teams a fast way to sanity-check the overhead that `lockman` adds to their use cases before they adopt it in production. The numbers here are not throughput targets — they are a calibration tool.

## What Was Measured

Two tracks run side by side:

- **memory-backed baseline** — uses `testkit.MemoryDriver` and `idempotency.NewMemoryStore`. This isolates the SDK's own overhead (type wiring, registry lookup, lease bookkeeping) from any network cost.
- **Redis-adapter-backed** — uses `backend/redis` and `idempotency/redis` backed by a local [miniredis](https://github.com/alicebob/miniredis) instance. This adds the Redis command round-trip and Lua script cost so you can see the relative overhead the adapter introduces.

Both tracks exercise `Run`, `Claim`, `Strict`, and `Composite` paths so you can compare the cost of each coordination mode.

## How To Run

```bash
go test -run '^$' -bench '^BenchmarkAdoption' -benchmem .
```

That single command runs every `BenchmarkAdoption*` function in both tracks. The `-benchmem` flag reports allocations per operation so you can spot GC pressure differences between the memory and Redis paths.

## Environment Notes

- **Memory-backed baseline**: runs entirely in-process. Numbers reflect Go runtime cost only. These will be consistent across machines with the same Go version.
- **Redis-adapter-backed**: uses miniredis, an in-process Redis simulator. It faithfully reproduces Lua script semantics and key expiry but does not model network latency or cross-process IPC. On real infrastructure expect the Redis track to be slower, not faster, than what miniredis reports.

## How To Read The Results

Focus on three things:

1. **Relative overhead** — compare the `Redis-adapter-backed` ns/op against the `memory-backed baseline` ns/op for the same benchmark. The ratio tells you how much adapter cost you are paying on top of SDK overhead. A ratio near 1.0 means the adapter adds almost nothing; a ratio of 3–5× is typical for Redis-backed locking.

2. **Contention shape** — the contention benchmarks show how the SDK behaves when two clients race for the same resource. If the Redis-backed contention numbers diverge sharply from the memory-backed numbers, your workload is sensitive to network latency under contention.

3. **Why you should not over-generalize** — these numbers come from a single miniredis process with no real network. Your production Redis cluster topology, replication lag, connection pooling, and p99 tail latency will all shift the absolute numbers. Use these benchmarks to compare SDK coordination modes against each other, not to predict production throughput.
