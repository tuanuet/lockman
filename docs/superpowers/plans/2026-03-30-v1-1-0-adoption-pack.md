# v1.1.0 Adoption Pack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an application-team-focused production guide plus a small benchmark suite and benchmark report that quantify the main `lockman` adoption choices without adding new primitives.

**Architecture:** Keep the implementation centered in the root module so both documentation and benchmarks are discoverable from the main SDK entry point. Use docs and benchmark surface tests to lock the public guidance and benchmark contract shape, then add root-module benchmarks in `package lockman_test` so the suite exercises the public SDK and can safely import `advanced/strict` and `advanced/composite` without creating import cycles. Use a stable memory-backed baseline plus a Redis-adapter-backed comparison track so one documented command can exercise the full adoption benchmark suite.

**Tech Stack:** Go 1.22, root-package Go tests and benchmarks, `lockkit/testkit` memory driver, `idempotency` memory store, `backend/redis`, `idempotency/redis`, `miniredis`, repository Markdown docs

---

## File Structure

- Create: `adoption_docs_surface_test.go`
  Purpose: lock the public documentation contract for the production guide, benchmark guide, and README links.
- Create: `docs/production-guide.md`
  Purpose: become the main application-team adoption guide with decision rules, minimum production wiring, and anti-patterns.
- Create: `docs/benchmarks.md`
  Purpose: document benchmark command(s), benchmark environment assumptions, and how to interpret the results.
- Create: `benchmark_adoption_surface_test.go`
  Purpose: lock the expected benchmark names and key semantic requirements before benchmark bodies are implemented.
- Create: `benchmark_adoption_helpers_test.go`
  Purpose: shared benchmark setup helpers for public-SDK `Run`, `Claim`, `strict`, and `composite` benchmark cases.
- Create: `benchmark_adoption_baseline_test.go`
  Purpose: stable baseline memory-backed benchmarks for the main adoption decisions.
- Create: `benchmark_adoption_adapter_test.go`
  Purpose: Redis-adapter-backed benchmarks using published adapter modules and `miniredis`.
- Modify: `README.md`
  Purpose: link the production guide and benchmark report from the main public entry point.
- Modify: `go.mod`
  Purpose: record new root-module test-only dependencies introduced by the benchmark files: `github.com/alicebob/miniredis/v2`, `github.com/redis/go-redis/v9`, `github.com/tuanuet/lockman/backend/redis`, and `github.com/tuanuet/lockman/idempotency/redis`.
- Modify: `go.sum`
  Purpose: capture sums for the new benchmark test dependencies.

## Commands To Reuse Throughout The Plan

- Targeted docs tests:
  `go test ./... -run 'TestProductionGuideForApplicationTeams|TestBenchmarkGuideForApplicationTeams|TestREADMELinksAdoptionDocs'`
- Benchmark suite command:
  `go test -run '^$' -bench '^BenchmarkAdoption' -benchmem .`
- Full repository verification:
  `go test ./...`
  `GOWORK=off go test ./...`

## Task 1: Lock And Publish The Application-Team Guidance

**Files:**
- Create: `adoption_docs_surface_test.go`
- Create: `docs/production-guide.md`
- Modify: `README.md`
- Test: `adoption_docs_surface_test.go`

- [ ] **Step 1: Write the failing docs surface test**

Create `adoption_docs_surface_test.go` in `package lockman_test` with these tests:

```go
package lockman_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProductionGuideForApplicationTeams(t *testing.T) {
	root := repoRoot(t)
	src := mustReadFile(t, filepath.Join(root, "docs", "production-guide.md"))

	for _, want := range []string{
		"# Production Guide",
		"## Start Here",
		"## Choose Run Or Claim",
		"## Minimum Production Wiring",
		"## Stay On The Default Path",
		"## When Strict Is Worth It",
		"## When Composite Is Worth It",
		"## TTL And Renewal Mindset",
		"## Identity And Ownership",
		"## Production Checklist",
		"## Common Mistakes",
		"## Which Example To Copy",
		"`Claim` requires idempotency wiring",
		"backend/redis",
		"idempotency/redis",
		"fail fast",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("expected production guide to contain %q", want)
		}
	}
}

func TestREADMELinksAdoptionDocs(t *testing.T) {
	root := repoRoot(t)
	src := mustReadFile(t, filepath.Join(root, "README.md"))

	for _, want := range []string{
		"docs/production-guide.md",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("expected README to contain %q", want)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(src)
}
```

- [ ] **Step 2: Run the docs test to verify it fails**

Run:

```bash
go test ./... -run 'TestProductionGuideForApplicationTeams|TestREADMELinksAdoptionDocs'
```

Expected: FAIL because `docs/production-guide.md` does not exist and the README does not link it yet.

- [ ] **Step 3: Write the minimal production guide and README link**

Create `docs/production-guide.md` with sections that directly answer application-team adoption questions. Include at minimum:

```md
# Production Guide

## Start Here

Start with one use case, one registry, one client, and one `Run(...)` or `Claim(...)` callsite.

## Choose Run Or Claim

- Use `Run` for direct request/response or synchronous orchestration.
- Use `Claim` for queue delivery, retries, or redelivery-aware work.

## Minimum Production Wiring

- `Run` requires a backend such as `github.com/tuanuet/lockman/backend/redis`.
- `Claim` requires both a backend and idempotency wiring such as `github.com/tuanuet/lockman/idempotency/redis`.
- Register all use cases at startup and fail fast on capability mismatches.

## Stay On The Default Path

Prefer the root SDK unless a concrete stale-writer or multi-resource requirement proves otherwise.
```

Continue the rest of the required sections with concrete recommendations, anti-recommendations, and links to the best matching examples under `examples/sdk/...` and published adapter examples.

Modify `README.md` so the main docs area includes a direct link to `docs/production-guide.md`.

- [ ] **Step 4: Run the docs test to verify it passes**

Run:

```bash
go test ./... -run 'TestProductionGuideForApplicationTeams|TestREADMELinksAdoptionDocs'
```

Expected: PASS

- [ ] **Step 5: Commit Task 1**

```bash
git add adoption_docs_surface_test.go docs/production-guide.md README.md
git commit -m "docs: add production guide for application teams"
```

## Task 2: Add The Baseline Benchmark Harness

**Files:**
- Create: `benchmark_adoption_surface_test.go`
- Create: `benchmark_adoption_helpers_test.go`
- Create: `benchmark_adoption_baseline_test.go`
- Modify: `go.mod`
- Modify: `go.sum`
- Test: `benchmark_adoption_surface_test.go`
- Test: `benchmark_adoption_helpers_test.go`
- Test: `benchmark_adoption_baseline_test.go`

- [ ] **Step 1: Write the failing benchmark surface test**

Create `benchmark_adoption_surface_test.go` in `package lockman_test`:

```go
package lockman_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBaselineBenchmarkContract(t *testing.T) {
	root := repoRoot(t)
	src := mustReadFile(t, filepath.Join(root, "benchmark_adoption_baseline_test.go"))

	for _, want := range []string{
		"package lockman_test",
		"func BenchmarkAdoptionRunMemory(",
		"func BenchmarkAdoptionRunContentionMemory(",
		"func BenchmarkAdoptionClaimMemory(",
		"func BenchmarkAdoptionClaimDuplicateMemory(",
		"func BenchmarkAdoptionStrictMemory(",
		"func BenchmarkAdoptionCompositeMemory(",
		"func BenchmarkAdoptionRenewalMemory(",
		"ErrDuplicate",
		"100 * time.Millisecond",
		"200 * time.Millisecond",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("expected baseline benchmark file to contain %q", want)
		}
	}
}
```

This creates a real red phase for the baseline benchmark contract before any benchmark implementation exists.

- [ ] **Step 2: Run the benchmark surface test to verify it fails**

Run:

```bash
go test ./... -run '^TestBaselineBenchmarkContract$'
```

Expected: FAIL because `benchmark_adoption_baseline_test.go` does not exist yet.

- [ ] **Step 3: Write the benchmark helpers and baseline benchmark declarations**

Create `benchmark_adoption_helpers_test.go` in `package lockman_test` with benchmark-local setup helpers. Do not reuse `mustRegisterUseCases` directly because it currently takes `*testing.T`, and do not depend on internal generic request types that do not exist on the public SDK surface. Include helpers along these lines:

```go
package lockman_test

import (
	"context"
	"testing"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/idempotency"
	"github.com/tuanuet/lockman/lockkit/testkit"
)

func registerBenchmarkRunUseCase(b *testing.B, reg *lockman.Registry, uc lockman.RunUseCase[string]) {
	b.Helper()
	if err := reg.Register(uc); err != nil {
		b.Fatalf("Register returned error: %v", err)
	}
}

func registerBenchmarkClaimUseCase(b *testing.B, reg *lockman.Registry, uc lockman.ClaimUseCase[string]) {
	b.Helper()
	if err := reg.Register(uc); err != nil {
		b.Fatalf("Register returned error: %v", err)
	}
}

func benchmarkRunUseCase(name string) lockman.RunUseCase[string] {
	return lockman.DefineRun[string](
		name,
		lockman.BindResourceID("order", func(v string) string { return v }),
	)
}

func newBenchmarkRunClient(b *testing.B, uc lockman.RunUseCase[string]) (*lockman.Client, lockman.RunRequest) {
	b.Helper()
	reg := lockman.NewRegistry()
	registerBenchmarkRunUseCase(b, reg, uc)
	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "bench-runner"}),
		lockman.WithBackend(testkit.NewMemoryDriver()),
	)
	if err != nil {
		b.Fatalf("New returned error: %v", err)
	}
	req, err := uc.With("123")
	if err != nil {
		b.Fatalf("With returned error: %v", err)
	}
	return client, req
}
```

Create `benchmark_adoption_baseline_test.go` in `package lockman_test` with the baseline benchmark entry points:

```go
package lockman_test

import "testing"

func BenchmarkAdoptionRunMemory(b *testing.B) {}
func BenchmarkAdoptionRunContentionMemory(b *testing.B) {}
func BenchmarkAdoptionClaimMemory(b *testing.B) {}
func BenchmarkAdoptionClaimDuplicateMemory(b *testing.B) {}
func BenchmarkAdoptionStrictMemory(b *testing.B) {}
func BenchmarkAdoptionCompositeMemory(b *testing.B) {}
func BenchmarkAdoptionRenewalMemory(b *testing.B) {}
```

Implement each benchmark with sub-benchmarks where helpful:

- `Run` uncontended loop calling `client.Run(...)`
- `Run` contention case with one holder and one busy competitor per iteration
- `Claim` uncontended loop with unique `MessageID`
- `Claim` duplicate case measuring the duplicate path after one successful claim, with the duplicate iteration asserting `errors.Is(err, lockman.ErrDuplicate)`
- `strict` path using `github.com/tuanuet/lockman/advanced/strict`
- `composite` path using `github.com/tuanuet/lockman/advanced/composite` with member counts `1`, `2`, and `4`
- renewal-heavy path with explicit timing values, using `lockman.TTL(100 * time.Millisecond)` and callback work of at least `200 * time.Millisecond` so renewal is required even on slower CI

- [ ] **Step 4: Run the benchmark surface test to verify the contract now passes**

Run:

```bash
go test ./... -run '^TestBaselineBenchmarkContract$'
```

Expected: PASS

- [ ] **Step 5: Run the benchmark command to verify the suite compiles and executes**

Run:

```bash
go test -run '^$' -bench '^BenchmarkAdoption' -benchmem .
```

Expected: PASS with benchmark output for all `BenchmarkAdoption...` baseline cases.

- [ ] **Step 6: Commit Task 2**

```bash
git add benchmark_adoption_surface_test.go benchmark_adoption_helpers_test.go benchmark_adoption_baseline_test.go go.mod go.sum
git commit -m "test(bench): add baseline adoption benchmarks"
```

## Task 3: Add The Redis-Backed Benchmark Track And Benchmark Guide

**Files:**
- Modify: `adoption_docs_surface_test.go`
- Modify: `benchmark_adoption_surface_test.go`
- Create: `benchmark_adoption_adapter_test.go`
- Create: `docs/benchmarks.md`
- Modify: `README.md`
- Modify: `go.mod`
- Modify: `go.sum`
- Test: `adoption_docs_surface_test.go`
- Test: `benchmark_adoption_surface_test.go`
- Test: `benchmark_adoption_adapter_test.go`

- [ ] **Step 1: Extend the docs surface test to require the benchmark guide contract**

Update `adoption_docs_surface_test.go` with a third test:

```go
func TestBenchmarkGuideForApplicationTeams(t *testing.T) {
	root := repoRoot(t)
	src := mustReadFile(t, filepath.Join(root, "docs", "benchmarks.md"))

	for _, want := range []string{
		"# Benchmarks",
		"## What Was Measured",
		"## How To Run",
		"## Environment Notes",
		"## How To Read The Results",
		"go test -run '^$' -bench '^BenchmarkAdoption' -benchmem .",
		"memory-backed baseline",
		"Redis-adapter-backed",
		"relative overhead",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("expected benchmark guide to contain %q", want)
		}
	}
}
```

Also extend `TestREADMELinksAdoptionDocs` to require `docs/benchmarks.md`.

Update `benchmark_adoption_surface_test.go` with a second test that locks the adapter-backed benchmark contract:

```go
func TestAdapterBenchmarkContract(t *testing.T) {
	root := repoRoot(t)
	src := mustReadFile(t, filepath.Join(root, "benchmark_adoption_adapter_test.go"))

	for _, want := range []string{
		"package lockman_test",
		"func BenchmarkAdoptionRunRedis(",
		"func BenchmarkAdoptionClaimRedis(",
		"func BenchmarkAdoptionStrictRedis(",
		"func BenchmarkAdoptionCompositeRedis(",
		"github.com/alicebob/miniredis/v2",
		"github.com/redis/go-redis/v9",
		"github.com/tuanuet/lockman/backend/redis",
		"github.com/tuanuet/lockman/idempotency/redis",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("expected adapter benchmark file to contain %q", want)
		}
	}
}
```

- [ ] **Step 2: Run the docs test to verify it fails**

Run:

```bash
go test ./... -run 'TestProductionGuideForApplicationTeams|TestBenchmarkGuideForApplicationTeams|TestREADMELinksAdoptionDocs|TestAdapterBenchmarkContract'
```

Expected: FAIL because `docs/benchmarks.md` and `benchmark_adoption_adapter_test.go` do not exist yet and the README does not link the benchmark guide.

- [ ] **Step 3: Add the Redis-adapter benchmark file**

Create `benchmark_adoption_adapter_test.go` in `package lockman_test` with adapter-backed cases that use the published adapter modules and `miniredis`:

```go
package lockman_test

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	backendredis "github.com/tuanuet/lockman/backend/redis"
	idempotencyredis "github.com/tuanuet/lockman/idempotency/redis"
)

func BenchmarkAdoptionRunRedis(b *testing.B) {}
func BenchmarkAdoptionClaimRedis(b *testing.B) {}
func BenchmarkAdoptionStrictRedis(b *testing.B) {}
func BenchmarkAdoptionCompositeRedis(b *testing.B) {}
```

Implementation requirements:

- start `miniredis` once per benchmark case
- wire root `Client` through `backend/redis`
- wire `Claim` through both `backend/redis` and `idempotency/redis`
- keep dependency additions explicit in `go.mod`/`go.sum` rather than leaving them implicit
- keep adapter-backed scope small: enough to prove production-path relevance without exploding runtime

- [ ] **Step 4: Write the benchmark guide and README link**

Create `docs/benchmarks.md` with:

- a short explanation of why these benchmarks exist
- the single command path:
  `go test -run '^$' -bench '^BenchmarkAdoption' -benchmem .`
- explicit environment notes for both memory-backed and Redis-adapter-backed tracks
- interpretation guidance that emphasizes relative overhead, contention shape, and why application teams should not over-generalize the numbers

Update `README.md` so it links to both:

- `docs/production-guide.md`
- `docs/benchmarks.md`

- [ ] **Step 5: Run the docs test to verify it passes**

Run:

```bash
go test ./... -run 'TestProductionGuideForApplicationTeams|TestBenchmarkGuideForApplicationTeams|TestREADMELinksAdoptionDocs|TestAdapterBenchmarkContract'
```

Expected: PASS

- [ ] **Step 6: Run the full benchmark command to verify both tracks compile and execute**

Run:

```bash
go test -run '^$' -bench '^BenchmarkAdoption' -benchmem .
```

Expected: PASS with both memory-backed and Redis-adapter-backed benchmark output.

- [ ] **Step 7: Commit Task 3**

```bash
git add adoption_docs_surface_test.go benchmark_adoption_surface_test.go benchmark_adoption_adapter_test.go docs/benchmarks.md README.md go.mod go.sum
git commit -m "docs(bench): add redis-backed benchmark guidance"
```

## Task 4: Final Adoption-Pack Verification And Cleanup

**Files:**
- Modify: `docs/production-guide.md`
- Modify: `docs/benchmarks.md`
- Modify: `README.md`
- Test: `adoption_docs_surface_test.go`
- Test: `benchmark_adoption_surface_test.go`
- Test: `benchmark_adoption_helpers_test.go`
- Test: `benchmark_adoption_baseline_test.go`
- Test: `benchmark_adoption_adapter_test.go`

- [ ] **Step 1: Re-read the new docs against the existing quickstarts and advanced docs**

Inspect:

```bash
sed -n '1,240p' docs/production-guide.md
sed -n '1,240p' docs/benchmarks.md
sed -n '1,220p' docs/runtime-vs-workers.md
sed -n '1,220p' docs/quickstart-sync.md
sed -n '1,220p' docs/quickstart-async.md
sed -n '1,220p' docs/advanced/strict.md
sed -n '1,220p' docs/advanced/composite.md
```

Expected: the production guide is the primary adoption doc, while the quickstarts and advanced docs still read as supporting references rather than contradictory guidance.

- [ ] **Step 2: Make any final wording-only cleanup required to remove ambiguity**

If you find wording conflicts, make the minimum doc edits needed. Do not expand scope into new feature work.

- [ ] **Step 3: Run the targeted docs test**

Run:

```bash
go test ./... -run 'TestProductionGuideForApplicationTeams|TestBenchmarkGuideForApplicationTeams|TestREADMELinksAdoptionDocs|TestBaselineBenchmarkContract|TestAdapterBenchmarkContract'
```

Expected: PASS

- [ ] **Step 4: Run the full repository verification**

Run:

```bash
go test ./...
GOWORK=off go test ./...
go test -run '^$' -bench '^BenchmarkAdoption' -benchmem .
```

Expected: PASS

- [ ] **Step 5: Commit Task 4**

```bash
git add docs/production-guide.md docs/benchmarks.md README.md adoption_docs_surface_test.go benchmark_adoption_surface_test.go benchmark_adoption_helpers_test.go benchmark_adoption_baseline_test.go benchmark_adoption_adapter_test.go go.mod go.sum
git commit -m "docs: finalize v1.1.0 adoption guidance"
```
