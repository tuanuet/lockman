# AGENTS Guide for `lockman`

This file is for autonomous coding agents working in this repository.
It documents the commands, conventions, and guardrails expected here.

## Repository Snapshot

- Language: Go (`go 1.22`)
- Root module: `github.com/tuanuet/lockman`
- Workspace file: `go.work`
- Multi-module workspace includes:
  - `.` (root SDK)
  - `./backend/redis` — Redis lease backend
  - `./benchmarks` — adoption benchmarks
  - `./examples` — SDK and core examples
  - `./guard/postgres` — Postgres guarded-write helpers
  - `./idempotency/redis` — Redis idempotency state
- CI runs tests in root and nested modules, plus external-consumer smoke checks.
- **SKILL.md** at repo root is the comprehensive API reference — consult it for SDK details.

## Primary Commands

Run from repository root unless noted otherwise.

### Build / Compile Checks

- Compile all packages via tests (no test execution):
  - `go test ./... -run '^$'`
- Compile tagged examples:
  - `go test -tags lockman_examples ./examples/... -run '^$'`

### Test Commands

- Full test suite (workspace mode):
  - `go test ./...`
- Full test suite without workspace mode (matches CI):
  - `GOWORK=off go test ./...`
- Module-specific suites (matches CI intent):
  - `go test ./backend/redis/...`
  - `go test ./idempotency/redis/...`
  - `go test ./guard/postgres/...`
- Makefile test target:
  - `make test`
- Documentation-linked tests:
  - `make test-docs`

### Run a Single Test (Important)

- Single test by exact name across all packages:
  - `go test ./... -run '^TestNewFailsWithoutRegistry$'`
- Single test in a specific package:
  - `go test . -run '^TestNewFailsWithoutRegistry$'`
  - `go test ./backend/redis -run '^TestRedisAcquire$'`
- Single subtest:
  - `go test . -run '^TestNewCreatesOnlyNeededManagers/run only$'`
- Verbose output while iterating:
  - `go test . -run '^TestName$' -v`

### Running Examples

- SDK examples (workspace mode, with build tag):
  - `go run -tags lockman_examples ./examples/sdk/sync-approve-order`
- With Redis backend:
  - `LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/sdk/sync-approve-order`
- Adapter examples (run from module root, no build tag):
  - `cd backend/redis && go run ./examples/sync-approve-order`

### Benchmarks

- All adoption benchmarks:
  - `make bench`
- Memory baseline benchmarks:
  - `make bench-baseline`
- Redis-backed benchmarks:
  - `make bench-redis`
- Direct benchmark invocation pattern:
  - `go test -run '^$' -bench '^BenchmarkName$' -benchmem ./benchmarks`

### Lint / Formatting / Hygiene

- Lint target from `Makefile`:
  - `make lint`
- Equivalent explicit commands:
  - `go vet ./...`
  - `gofmt -l .`
- Apply formatting changes:
  - `gofmt -w .`
- Dependency/workspace sync:
  - `make tidy`

## CI Parity Checklist (Before PR)

Run these locally before claiming completion:

1. `go test ./...`
2. `GOWORK=off go test ./...`
3. `go test ./backend/redis/...`
4. `go test ./idempotency/redis/...`
5. `go test ./guard/postgres/...`
6. `go test -tags lockman_examples ./examples/... -run '^$'`

## Local Development Infrastructure

### Docker Compose

- Start Redis and Postgres for integration testing:
  - `docker compose up -d`
- Redis: `localhost:6379` (override via `LOCKMAN_REDIS_PORT`)
- Postgres: `localhost:5432`, db `lockman`, user `postgres`, password `postgres` (override via `LOCKMAN_POSTGRES_PORT`)

### Environment Variables

- `LOCKMAN_REDIS_URL` — Redis connection string for examples (e.g. `redis://localhost:6379/0`)
- `LOCKMAN_REDIS_PORT` — Override Redis port in docker-compose
- `LOCKMAN_POSTGRES_PORT` — Override Postgres port in docker-compose

## Release Process

### Tagging

Each module is versioned independently. Push tags to trigger CI verification and GitHub release:

| Module | Tag pattern |
|--------|-------------|
| Root SDK | `v1.x.x` |
| backend/redis | `backend/redis/v1.x.x` |
| idempotency/redis | `idempotency/redis/v1.x.x` |
| guard/postgres | `guard/postgres/v1.x.x` |

### Release CI (release.yml)

1. **verify** — runs full test suite across all modules (same as CI)
2. **external-consumer** — smoke tests module installation from proxy.golang.org in an isolated temp directory
3. **publish** — creates GitHub release with auto-generated notes (softprops/action-gh-release)

### Release Checklist

1. Update `CHANGELOG.md` under `[Unreleased]` section
2. Run full CI parity checklist (see above)
3. Create and push tag: `git tag v1.x.x && git push origin v1.x.x`
4. Verify GitHub release was created automatically
5. For adapter releases, tag the specific module path: `git tag backend/redis/v1.x.x && git push origin backend/redis/v1.x.x`

### External Consumer Smoke Tests

- CI verifies modules install outside the repo via `testdata/externalconsumer/smoke_test.go`
- Release smoke tests live in `testdata/releaseconsumer/` (per-module)
- These ensure consumers can `go get` and compile against published versions

## CI Pipelines

### ci.yml (PR + main)

- **test** job: runs all module tests + compiles tagged examples
- **external-consumer** job: verifies `go get` + compile against `v1.0.0` published modules

### release.yml (tags)

- Triggered on `v*`, `backend/redis/v*`, `idempotency/redis/v*`, `guard/postgres/v*`
- Verifies + smoke tests + publishes GitHub release

## Key Internal Packages

| Package | Purpose |
|---------|---------|
| `lockkit/runtime` | Sync exclusive execution engine |
| `lockkit/holds` | Detached hold acquire/release manager |
| `lockkit/workers` | Async claim with renewal loop |
| `lockkit/definitions` | Canonical lock models |
| `lockkit/registry` | Use case storage + validation |
| `lockkit/errors` | Internal sentinels (normalized to root errors) |
| `lockkit/observe` | Lifecycle event hooks |
| `lockkit/internal/lineage` | Ancestor chains, lease IDs |
| `lockkit/internal/policy` | Outcome mapping, overlap rejection, composite canonicalization |
| `internal/sdk` | Bridge: normalizes public use cases/requests into internal forms |
| `backend/memory` | In-memory backend for unit testing |

## Code Style Guidelines

Follow existing repository style over personal preference.

### Formatting and File Layout

- Always use `gofmt` formatting (tabs, canonical spacing, import order).
- Keep files focused; avoid unrelated refactors in functional changes.
- Prefer small, composable functions over monolithic blocks.
- Keep package names short, lowercase, and noun-like.

### Imports

- Group imports in Go standard style:
  1. Standard library
  2. Blank line
  3. Internal/external module imports
- Avoid unused imports; run `go test` after edits.
- Use import aliases only when necessary for clarity or conflicts.

### Naming Conventions

- Exported identifiers: `PascalCase` (`Client`, `DefineRun`, `RunRequest`).
- Unexported identifiers: `camelCase` (`useCaseCore`, `buildClientPlan`).
- Constants: `camelCase` for unexported, `PascalCase` for exported.
- Error vars: exported sentinel errors use `ErrXxx` pattern.
- Test names: `TestXxx...`; benchmark names: `BenchmarkXxx...`.

### Types and API Shape

- Use strong typing and generics where APIs already do (for use cases/binding).
- Keep public API stable and explicit; avoid changing exported signatures lightly.
- Prefer typed structs/options over `map[string]any` in public-facing code.
- Keep request structs opaque externally when that is already the pattern.

### Error Handling

- Prefer sentinel errors for stable categories (`ErrBusy`, `ErrIdentityRequired`, etc.).
- Wrap errors with context using `%w`:
  - `fmt.Errorf("lockman: <context>: %w", err)`
- Keep error messages lowercase and without trailing punctuation.
- Keep the `lockman:` prefix for SDK-originated error text where established.
- For branching, use `errors.Is` / `errors.As` instead of string matching.

### Control Flow and Concurrency

- Return early on validation failures.
- Keep shutdown/cleanup paths explicit and deterministic.
- When combining cleanup failures, preserve all meaningful errors.
- Avoid introducing shared mutable state without clear synchronization.

### Testing Expectations

- Add/adjust tests in the closest relevant package.
- Prefer targeted unit tests first, then broader integration coverage when needed.
- Use precise `-run` patterns while iterating; run wider suites before finishing.
- Use clear failure messages with expected vs actual behavior.
- For error assertions, prefer `errors.Is` for sentinel compatibility.

## Documentation Expectations

- Update docs/README when changing public behavior, module usage, or workflows.
- Keep examples aligned with current APIs.
- If adding commands, ensure they are reproducible from repo root.

## Agent Workflow Guidelines

- Start by reading `Makefile`, `README.md`, and affected package files.
- Make minimal, scoped changes directly related to the requested task.
- Do not fix unrelated issues unless explicitly asked.
- Before finishing, run relevant tests and linters for touched areas.
- Prefer evidence-backed claims (include exactly what command was run).

## Cursor / Copilot Rules

Agent scan result in this repository:

- No `.cursorrules` file found.
- No files under `.cursor/rules/` found.
- No `.github/copilot-instructions.md` found.

If any of these rule files are added later, treat them as high-priority instructions and update this `AGENTS.md` summary.
