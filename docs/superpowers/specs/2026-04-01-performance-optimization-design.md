# Performance Optimization Design

**Date:** 2026-04-01
**Scope:** Internal SDK, Redis driver, concurrency primitives — public API unchanged.

## Problem

A deep performance analysis identified nine sources of unnecessary CPU and allocation overhead on the hot path. These matter most for high-throughput services where lock operations per second are in the thousands. The dominant production cost is always the Redis round-trip; this design eliminates avoidable SDK-side overhead on top of that.

## Goals

- Fix all identified hot-path inefficiencies
- Public-facing API (`client.Run`, `client.Claim`, `Binding`, etc.) stays unchanged
- Add targeted benchmarks to validate each improvement
- Deliver in three independently reviewable layers

## Out of Scope

- Redis cluster topology, connection pooling, or network tuning
- Public API changes
- Unrelated refactoring

---

## Layer 1: SDK Core

### 1.1 Replace `activeCount()` O(N) scan with atomic counters

**File:** `lockkit/runtime/manager.go`, `lockkit/runtime/exclusive.go`

**Problem:** `recordActiveLocks` calls `activeCount`, which does `sync.Map.Range` over all active guards across all definitions. Called twice per lock cycle (after acquire, after release). O(N) where N = total active locks across all definitions.

**Fix:** Add `activeByDef sync.Map` on `Manager` mapping `definitionID → *atomic.Int64`. Increment on acquire, decrement on release. `recordActiveLocks` reads the counter directly — O(1).

**Benchmark:** `BenchmarkActiveCount` — parallel goroutines holding locks, measure cost per operation.

### 1.2 Eliminate `definitionsByID()` per-call rebuild

**File:** `lockkit/runtime/exclusive.go`, `lockkit/runtime/manager.go`, `lockkit/workers/execute.go`, `lockkit/workers/manager.go`

**Problem:** `buildAcquirePlan` (runtime) and `buildClaimAcquirePlan` (workers) both call `definitionsByID()` on every operation. This rebuilds a map from scratch: takes an RLock, iterates all definitions, deep-clones each one (including Tags map), allocates a new `map[string]LockDefinition`. Used only to check lineage (`ParentRef != ""` and `len(children[def.ID]) > 0`). The workers path at `workers/execute.go:354-373` has the identical pattern.

**Fix:** At `NewManager` time, walk all definitions once and build two cached structures:

1. `lineageDefs map[string]bool` — definition ID → uses lineage. `buildAcquirePlan` checks this map for the non-lineage fast path — zero allocations.
2. `cachedDefsByID map[string]LockDefinition` — the full definitions-by-ID map. For the lineage case, `lineage.ResolveAcquirePlan` receives this cached map instead of a freshly rebuilt one. Definitions are immutable after `Validate()`, so this is safe.

**Benchmark:** `BenchmarkBuildAcquirePlan` — isolate plan-building cost.

### 1.3 Remove defensive cloning in `MustGet`

**File:** `lockkit/registry/registry.go`

**Problem:** `MustGet` deep-clones the definition (including Tags map) on every read. Definitions are immutable after `Validate()` — cloning is unnecessary.

**Fix:** Add `Get(id string) (LockDefinition, bool)` to Registry that returns the stored value directly without cloning, under RLock. Both runtime Manager and worker Manager call `Get` instead of `MustGet`. Remove the `defer/recover` wrapper in `getDefinition` (present in both `lockkit/runtime/exclusive.go:228-236` and `lockkit/workers/manager.go:175-183`) — replace with a plain `ok` check in both.

### 1.4 Optimize `TemplateKeyBuilder.Build`

**File:** `lockkit/definitions/key_builder.go`

**Problem:** Every call to `Build` allocates a `[]string` for replacements, constructs placeholder strings with `"{"+ field +"}"`, and creates a new `strings.Replacer`. Three to five allocations per key build.

**Fix:** Pre-compute placeholder strings at construction time. For the single-field case (most common), use `strings.Replace` directly — no Replacer allocation. For multi-field, reuse the pre-computed placeholder strings to avoid repeated string construction.

**Benchmark:** `BenchmarkKeyBuilderBuild` — single-field and multi-field cases with `-benchmem`.

---

## Layer 2: Redis Driver

### 2.1 Cache `encodeSegment` results

**File:** `backend/redis/driver.go`

**Problem:** `encodeSegment` calls `base64.RawURLEncoding.EncodeToString([]byte(v))` on every invocation, allocating on every call. Definition IDs are a small, stable set encoded thousands of times per second. Resource keys, however, are user-provided and unique per operation — a global cache for resource keys would grow without bound.

**Fix:** Cache encoded segments at the `Driver` level, scoped to definition IDs only. Add an optional `WithDefinitionIDs(ids []string)` configuration function (or accept them via `NewDriver`) to pre-encode definition IDs into a `map[string]string` at construction time. Key-building methods check this map first; resource keys are encoded inline as before. This eliminates allocations for the definition ID component (2 of 4 per key build) without unbounded cache growth. Note: this changes the `NewDriver` constructor or adds a configuration option — both are internal API, not public.

**Benchmark:** `BenchmarkRedisKeyBuild` — measure allocations per `buildLeaseKey` call.

### 2.2 Replace `fmt.Sprintf` with direct concatenation for key construction

**File:** `backend/redis/driver.go`

**Problem:** `buildLeaseKey`, `buildStrictFenceCounterKey`, `buildStrictTokenKey`, and `buildLineageKey` all use `fmt.Sprintf`. With cached encoded segments (from 2.1), the segments are already strings — `fmt.Sprintf` adds unnecessary format-string parsing and internal buffer allocation.

**Fix:** Replace with `strings.Builder` or direct `+` concatenation. Combined with 2.1, the key-building path becomes effectively allocation-free.

### 2.3 Tighten Lua scripts

**File:** `backend/redis/scripts.go`

One minor change:

- **`lineageRenewScript`:** The two-pass ancestor structure (validate all with `ZSCORE`, then update all with `ZADD`) is intentional — it prevents partial updates when validation fails mid-way. However, the `PEXPIRE` call on the lease key (line 188) currently runs between the two loops. If the script errors mid-update-loop (e.g., Redis OOM), the lease TTL has already been extended without corresponding lineage updates. Move `PEXPIRE` after the update loop for consistency. This is an edge-case improvement — Lua scripts execute atomically, so it only matters if the script itself errors partway through.

Note: the `EXISTS` check in `strictAcquireScript` was initially identified as redundant with `SET NX`, but it intentionally guards the `INCR` on the fencing counter — without it, every contended acquire would increment the counter, wasting token space and changing counter semantics. The `EXISTS` check stays.

---

## Layer 3: Concurrency Primitives

### 3.1 Replace `lifecycleMu` with atomics on hot path

**File:** `lockkit/runtime/manager.go`, `lockkit/workers/manager.go`

**Problem:** `tryAdmitInFlightExecution` and `releaseInFlightExecution` take `lifecycleMu` on every lock acquire and release. This pattern exists in both the runtime Manager (`runtime/manager.go:88-115`) and the worker Manager (`workers/manager.go:108-133`). Under high concurrency this serializes admission.

**Fix:** Replace `inFlight int` with `atomic.Int64` in both managers. The shutdown drain channel is managed via a `sync.Mutex` + `sync.Cond` only on the rare shutdown path — the hot path never takes a mutex. `shuttingDown atomic.Bool` stays as-is.

### 3.2 Shard `MemoryStore` mutex

**File:** `idempotency/memory_store.go`

**Problem:** Single `sync.Mutex` serializes all idempotency operations (`Begin`, `Get`, `Complete`, `Fail`). Concurrent claims on different keys block each other unnecessarily.

**Fix:** Replace with 16 shards, each holding a `sync.Mutex` and `map[string]Record`. Shard selection: `hash(key) % 16`. Public API stays identical. This is primarily relevant for benchmark accuracy (the memory store is used in benchmarks) and for deployments that use `MemoryStore` in production (e.g., integration tests, local runs).

---

## Benchmark Strategy

Run before and after each layer with:

```bash
go test -run '^$' -bench '^BenchmarkAdoption' -benchmem -count=5 . > before.txt
# apply changes
go test -run '^$' -bench '^BenchmarkAdoption' -benchmem -count=5 . > after.txt
benchstat before.txt after.txt
```

New benchmarks to add:

| Benchmark                         | Measures                                   |
| --------------------------------- | ------------------------------------------ |
| `BenchmarkActiveCount`            | `activeCount` cost under concurrent load   |
| `BenchmarkBuildAcquirePlan`       | `definitionsByID` + lineage check overhead |
| `BenchmarkKeyBuilderBuild/single` | Single-field template key build            |
| `BenchmarkKeyBuilderBuild/multi`  | Multi-field template key build             |
| `BenchmarkRedisKeyBuild`          | `buildLeaseKey` allocations                |

Focus metric: `allocs/op` for SDK overhead, `ns/op` for the memory-backed baseline (isolates SDK cost from Redis cost).

---

## Expected Allocation Reduction

For a standard `Run()` call (non-lineage, single-field key):

| Source                    | Before             | After              |
| ------------------------- | ------------------ | ------------------ |
| `MustGet` clone           | 2 allocs           | 0                  |
| `definitionsByID` rebuild | 1 + D allocs       | 0                  |
| `KeyBuilder.Build`        | 4 allocs           | 1                  |
| `buildLeaseKey` (Redis)   | 4 allocs           | 2                  |
| `activeCount`             | 0 allocs, O(N) CPU | 0 allocs, O(1) CPU |
| **Total**                 | ~12 + D            | ~4                 |

D = number of registered definitions.
