# SKILL.md — lockman AI Reference

Comprehensive reference for the `lockman` Go SDK. Use this when implementing, debugging, or extending lockman-based code.

---

## 1. Mental Model

lockman uses a **definition-first** model:

```
LockDefinition  →  Execution Surface(s)  →  Registry  →  Client  →  Run/Hold/Claim
   (boundary)        (Run/Hold/Claim)         (wire)      (exec)     (execute)
```

1. **DefineLock** — declare a shared lock boundary once, with typed input binding
2. **DefineRunOn / DefineHoldOn / DefineClaimOn** — attach execution surfaces to that boundary
3. **Registry** — register all use cases centrally at startup
4. **Client** — construct with backend + identity + optional adapters
5. **Execute** — call `Run`, `Hold`, `Claim`, or `Forfeit` through the client

---

## 2. Public SDK APIs

### 2.1 DefineLock

```go
func DefineLock[T any](name string, binding Binding[T], opts ...DefinitionOption) LockDefinition[T]
```

Creates a shared lock definition. The definition owns the lock identity and binding logic.

**Options:**
- `StrictDef()` — marks the definition as requiring strict fenced execution (fencing tokens)

### 2.2 Execution Surfaces

```go
func DefineRunOn[T any](name string, def LockDefinition[T], opts ...UseCaseOption) RunUseCase[T]
func DefineHoldOn[T any](name string, def LockDefinition[T], opts ...UseCaseOption) HoldUseCase[T]
func DefineClaimOn[T any](name string, def LockDefinition[T], opts ...UseCaseOption) ClaimUseCase[T]
```

**Use-case options:**
- `TTL(time.Duration)` — lease TTL hint
- `WaitTimeout(time.Duration)` — acquire wait budget
- `Idempotent()` — required for claim use cases that must deduplicate deliveries

### 2.3 Composite Locks

```go
func DefineCompositeRun[T any](name string, members ...CompositeMember[T]) RunUseCase[T]
func DefineCompositeRunWithOptions[T any](name string, opts []UseCaseOption, members ...CompositeMember[T]) RunUseCase[T]
```

**Member builders:**
```go
func Member[TInput, TMember](def LockDefinition[TMember], keyFn func(TInput) string) CompositeMember[TInput]
func MemberWithStrict[TInput, TMember](def LockDefinition[TMember], keyFn func(TInput) string) CompositeMember[TInput]
```

Use when a single operation must hold multiple resources together atomically.

### 2.4 Binding Helpers

```go
func BindResourceID[T any](resource string, fn func(T) string) Binding[T]  // normalizes to "resource:<id>"
func BindKey[T any](fn func(T) string) Binding[T]                            // uses caller-provided key directly
```

Prefer `BindResourceID` for single-resource use cases. Use `BindKey` only when the key shape is genuinely custom.

### 2.5 Call-time Overrides

```go
func OwnerID(id string) CallOption  // overrides owner identity for a single call
```

### 2.6 Client Construction

```go
func New(opts ...ClientOption) (*Client, error)
```

**Client options:**
- `WithIdentity(Identity)` — static owner identity
- `WithIdentityProvider(func(context.Context) Identity)` — dynamic identity provider
- `WithRegistry(*Registry)` — required: central use case registry
- `WithBackend(backend.Driver)` — required: lease backend (e.g. `backend/redis`)
- `WithIdempotency(idempotency.Store)` — required for `Claim` use cases
- `WithObserver(observe.Dispatcher)` — async event export
- `WithInspectStore(*inspect.Store)` — process-local debug state
- `WithObservability(Observability)` — bundles dispatcher + inspect store

### 2.7 Client Methods

```go
func (c *Client) Run(ctx context.Context, req RunRequest, fn func(context.Context, Lease) error) error
func (c *Client) Hold(ctx context.Context, req HoldRequest) (HoldHandle, error)
func (c *Client) Claim(ctx context.Context, req ClaimRequest, fn func(context.Context, Claim) error) error
func (c *Client) Forfeit(ctx context.Context, req ForfeitRequest) error
func (c *Client) Shutdown(ctx context.Context) error
```

### 2.8 Registry

```go
func NewRegistry() *Registry
func (r *Registry) Register(useCases ...registeredUseCase) error
```

Register all use cases before creating the client. The client validates the registry at construction.

### 2.9 Key Types

```go
type Identity struct { OwnerID, Service, Instance string }

type Lease struct {
    UseCase       string
    ResourceKey   string
    ResourceKeys  []string
    LeaseTTL      time.Duration
    Deadline      time.Time
    FencingToken  int64  // only in strict mode
}

type Claim struct {
    UseCase         string
    ResourceKey     string
    LeaseTTL        time.Duration
    Deadline        time.Time
    FencingToken    int64
    IdempotencyKey  string
}

type Delivery struct { MessageID, ConsumerGroup string; Attempt int }

type HoldHandle struct { /* opaque */ }
func (h HoldHandle) Token() string
```

---

## 3. Execution Surfaces — When to Use Which

### Run (sync critical section)

Use for request/response handlers or job orchestration where mutual exclusion is needed within a single callback.

```go
req, _ := Approve.With(ApproveInput{OrderID: "123"})
err := client.Run(ctx, req, func(ctx context.Context, lease lockman.Lease) error {
    return doWork(ctx)
})
```

### Hold (manual retention)

Use when a user or process must retain a lock across multiple steps, not just one callback.

```go
// Acquire
req, _ := ManualHold.With(holdInput{OrderID: "123"})
handle, err := client.Hold(ctx, req)
token := handle.Token()

// Later: Forfeit
err = client.Forfeit(ctx, ManualHold.ForfeitWith(token))
```

### Claim (async idempotent processing)

Use when work starts from message delivery/retry/redelivery and needs deduplication.

```go
req, _ := Process.With(ProcessInput{OrderID: "123"}, lockman.Delivery{
    MessageID: "msg-1", ConsumerGroup: "orders", Attempt: 1,
})
err := client.Claim(ctx, req, func(ctx context.Context, claim lockman.Claim) error {
    return processOrder(ctx)
})
```

---

## 4. Error Sentinels

Use `errors.Is()` for sentinel compatibility. Never string-match error messages.

### Root package (`github.com/tuanuet/lockman`)

| Sentinel | When it occurs |
|----------|---------------|
| `ErrBusy` | Resource already locked |
| `ErrTimeout` | Acquire wait timed out |
| `ErrDuplicate` | Idempotent delivery already processed |
| `ErrOverlapRejected` | Child lock overlaps with parent (lineage violation) |
| `ErrShuttingDown` | Client/manager is shutting down |
| `ErrLeaseLost` | Lease expired or revoked during execution |
| `ErrInvariantRejected` | Guard write invariant violated |
| `ErrUseCaseNotFound` | Registry has no matching use case |
| `ErrRegistryMismatch` | Use case not registered in this client's registry |
| `ErrRegistryRequired` | Client created without registry |
| `ErrIdentityRequired` | Client created without identity |
| `ErrBackendRequired` | Client created without backend |
| `ErrHoldTokenInvalid` | Forfeit called with invalid token |
| `ErrHoldExpired` | Hold lease expired before forfeit |
| `ErrBackendCapabilityRequired` | Operation needs backend capability not present |
| `ErrIdempotencyRequired` | Claim use case needs idempotency store |
| `ErrNotImplemented` | Feature not implemented in this backend |

### Lower-level sentinels (internal/adapters)

- `backend.ErrInvalidRequest`, `backend.ErrLeaseAlreadyHeld`, `backend.ErrOverlapRejected`, `backend.ErrLeaseNotFound`, `backend.ErrLeaseExpired`, `backend.ErrLeaseOwnerMismatch`
- `lockkit/errors.ErrLockBusy`, `ErrLockAcquireTimeout`, `ErrLeaseLost`, `ErrRegistryViolation`, `ErrPolicyViolation`, `ErrReentrantAcquire`, `ErrDuplicateIgnored`, `ErrWorkerShuttingDown`
- `guard.ErrInvariantRejected`

---

## 5. Advanced Features

### 5.1 Strict Fenced Execution

Use when you need monotonic fencing tokens to protect stale writers or implement compare-and-swap on persistence.

```go
strictDef := lockman.DefineLock(
    "order.strict-write",
    lockman.BindResourceID("order", func(in Input) string { return in.OrderID }),
    lockman.StrictDef(),
)

approve := lockman.DefineRunOn("order.strict-write", strictDef)

req, _ := approve.With(Input{OrderID: "123"})
err := client.Run(ctx, req, func(ctx context.Context, lease lockman.Lease) error {
    log.Println("fencing token:", lease.FencingToken)  // monotonic, increases on each reacquire
    return nil
})
```

- Backend must implement `backend.StrictDriver` capability
- Fencing tokens increase monotonically across successive acquires on the same boundary
- Example: [`examples/sdk/sync-fenced-write`](examples/sdk/sync-fenced-write)
- Deep core examples: [`examples/core/strict-sync-fencing`](examples/core/strict-sync-fencing), [`examples/core/strict-async-fencing`](examples/core/strict-async-fencing)
- Docs: [`docs/advanced/strict.md`](docs/advanced/strict.md)

### 5.2 Composite Locking

Use when a single operation must atomically lock multiple resources together.

```go
uc := lockman.DefineCompositeRun(
    "transfer",
    lockman.Member(fromDef, func(in TransferInput) string { return in.FromID }),
    lockman.Member(toDef, func(in TransferInput) string { return in.ToID }),
)
```

- Members are canonicalized (ordered by rank, resource, resource key)
- Composite execution acquires all members atomically
- Example: [`examples/core/sync-composite-lock`](examples/core/sync-composite-lock)
- Docs: [`docs/advanced/composite.md`](docs/advanced/composite.md)

### 5.3 Lineage and Overlap Rules

Parent-child lock semantics with explicit overlap rejection.

- Parent locks can reject child acquires that overlap their keys
- Cyclic parent detection and missing-parent detection are enforced
- Backend can implement `backend.LineageDriver` for lineage-aware semantics
- Example: [`examples/core/parent-child-lineage`](examples/core/parent-child-lineage), [`examples/core/parent-child-overlap`](examples/core/parent-child-overlap)
- Docs: [`docs/advanced/lineage.md`](docs/advanced/lineage.md)

### 5.4 Guarded Writes

Integrate fencing tokens into database writes to detect stale writers and boundary mismatches.

```go
// Inside a strict Run callback:
ctx := guard.WithContext(ctx, lease.FencingToken, ...)
outcome, err := guard.ClassifyExistingRowUpdate(ctx, rowStatus)
// outcome: applied, stale_rejected, or invariant_rejected
```

- `guard.Context` carries fencing metadata into the persistence layer
- `guard.Outcome` maps the persistence decision into explicit outcomes
- `guard.ErrInvariantRejected` when the write violates a guard invariant
- Example: [`examples/core/strict-guarded-write`](examples/core/strict-guarded-write)
- Adapter: [`guard/postgres`](guard/postgres) with `ScanExistingRowStatus` and `ClassifyExistingRowUpdate`
- Docs: [`docs/advanced/guard.md`](docs/advanced/guard.md)

---

## 6. Adapter Modules

### 6.1 backend/redis

```go
func New(client goredis.UniversalClient, keyPrefix string) backend.Driver
```

**Capabilities:**
- `backend.Driver` — standard lease acquire/renew/release
- `backend.StrictDriver` — strict fencing token support
- `backend.LineageDriver` — lineage-aware acquire/renew/release
- `backend.ForceReleaseDriver` — force release by definition/resource key

**Internal mechanics:**
- Uses Redis Lua scripts for atomicity
- Default key namespace: `lockman:lease`
- Supports presence checks

**Examples:**
- [`backend/redis/examples/sync-approve-order`](backend/redis/examples/sync-approve-order)
- [`backend/redis/examples/sync-transfer-funds`](backend/redis/examples/sync-transfer-funds)
- [`backend/redis/examples/sync-fenced-write`](backend/redis/examples/sync-fenced-write)

### 6.2 idempotency/redis

```go
func New(client goredis.UniversalClient, keyPrefix string) idempotency.Store
```

**Operations:** `Get`, `Begin`, `Complete`, `Fail`

**States:** `missing` → `in_progress` → `completed` or `failed`

- Uses Lua scripts to avoid races
- Terminal states have TTL
- Default key namespace: `lockman:idempotency`

**Example:** [`idempotency/redis/examples/async-process-order`](idempotency/redis/examples/async-process-order)

### 6.3 guard/postgres

```go
func ScanExistingRowStatus(scanner rowScanner) (ExistingRowStatus, error)
func ClassifyExistingRowUpdate(g guard.Context, status ExistingRowStatus) (guard.Outcome, error)
```

Classifies row updates into: `applied`, `stale_rejected`, `invariant_rejected`.

---

## 7. Observability

### 7.1 Event Dispatcher (`observe` package)

```go
dispatcher := observe.NewDispatcher(
    observe.WithBufferSize(1024),
    observe.WithSink(mySink),
    observe.WithExporter(myExporter),
    observe.WithWorkerCount(4),
)
```

**Event kinds:** acquire started/succeeded/failed, released, contention, overlap, lease lost, renewal succeeded/failed, shutdown started/completed, client started, overlap rejected, presence checked.

**Sinks/Exporters:**
- `observe.NoopSink`, `observe.NoopExporter`, `observe.NoopDispatcher`
- `observe.NewOTelSink(...)` — OpenTelemetry adapter

**Methods:** `Publish`, `Shutdown`, `DroppedCount`, `SinkFailureCount`, `ExporterFailureCount`

### 7.2 Inspect Store (`inspect` package)

```go
store := inspect.NewStore(inspect.WithHistoryLimit(100))
handler := inspect.NewHandler(store, inspect.WithPrefix("/locks/inspect"))
http.Handle("/locks/", handler)
```

**HTTP endpoints** (default prefix `/locks/inspect`):
- `GET /locks/inspect` — snapshot
- `GET /locks/inspect/active` — active runtime locks
- `GET /locks/inspect/events` — recent event history
- `GET /locks/inspect/health` — pipeline health
- `GET /locks/inspect/stream` — SSE event stream

**Key points:**
- Inspection data is process-local, not cluster truth
- Export failures do not fail the lock lifecycle
- Dispatcher operates on best-effort basis

**Example:** [`examples/sdk/observability-basic`](examples/sdk/observability-basic)

---

## 8. Internal Architecture (lockkit)

The public SDK translates into internal `lockkit` execution engines:

| Public surface | Internal engine | Path |
|---------------|-----------------|------|
| `Run` | `lockkit/runtime.Manager` — sync exclusive execution | `lockkit/runtime` |
| `Hold` | `lockkit/holds.Manager` — detached acquire/release | `lockkit/holds` |
| `Claim` | `lockkit/workers.Manager` — async claim with renewal loop | `lockkit/workers` |
| Definitions | `lockkit/definitions` — canonical lock models | `lockkit/definitions` |
| Registry | `lockkit/registry` — storage + validation | `lockkit/registry` |
| Lineage | `lockkit/internal/lineage` — ancestor chains, lease IDs | `lockkit/internal/lineage` |
| Policy | `lockkit/internal/policy` — outcome mapping, overlap rejection, composite canonicalization | `lockkit/internal/policy` |
| Errors | `lockkit/errors` — internal sentinels normalized to root errors | `lockkit/errors` |
| Observe | `lockkit/observe.Recorder` — lifecycle event hooks | `lockkit/observe` |
| Test support | `lockkit/testkit.MemoryDriver` — in-memory backend for tests | `lockkit/testkit` |

**Bridge:** `internal/sdk` normalizes public use cases/requests into internal forms before dispatching to lockkit managers.

---

## 9. Example Catalog

### SDK examples (`examples/sdk/`) — start here

| Example | Demonstrates |
|---------|-------------|
| `shared-lock-definition` | Canonical definition-first starter (Run + Hold on same boundary) |
| `sync-approve-order` | Shortest sync `Run` flow |
| `manual-hold` | Hold acquire + forfeit flow |
| `async-process-order` | Async `Claim` with idempotency |
| `shared-aggregate-split-definitions` | Sync + async surfaces over one aggregate boundary |
| `parent-lock-over-composite` | When one parent lock is enough (composite is overkill) |
| `sync-transfer-funds` | Multi-resource sync lock (transfer semantics) |
| `sync-fenced-write` | Strict fenced execution on SDK path |
| `observability-basic` | Observability + inspect wiring |

### Core examples (`examples/core/`) — deeper follow-up

| Example | Demonstrates |
|---------|-------------|
| `strict-sync-fencing` | Strict sync runtime with fencing token visibility |
| `strict-async-fencing` | Strict async worker with fencing + idempotency |
| `strict-guarded-write` | Strict fencing carried into guarded DB write |
| `sync-composite-lock` | Sync composite multi-resource locking |
| `async-composite-lock` | Async composite claim flow |
| `composite-overlap-reject` | Overlap rejection in composite scenarios |
| `parent-child-lineage` | Lineage/ancestry semantics |
| `parent-child-overlap` | Parent/child overlap rule enforcement |
| `lease-ttl-expiry` | Lease TTL expiry behavior |
| `sync-reentrant-reject` | Re-entrant acquire rejection |
| `sync-lock-contention` | Lock contention scenario |
| `sync-single-resource` | Basic single-resource sync lock |
| `async-single-resource` | Basic single-resource async claim |
| `async-bulk-import-shard` | Bulk import sharded async scenario |
| `shared-definition-contention` | Contention over shared definition |
| `shared-aggregate-split-definitions` | Shared aggregate with split definitions |
| `parent-lock-over-composite` | Aggregate-over-composite reasoning |
| `observability-runtime` | Runtime-level observability/inspection |

### Adapter examples (run from module root, no build tag)

| Path | Demonstrates |
|------|-------------|
| `backend/redis/examples/sync-approve-order` | Redis-backed sync approve |
| `backend/redis/examples/sync-transfer-funds` | Redis-backed multi-resource transfer |
| `backend/redis/examples/sync-fenced-write` | Redis-backed strict fenced write |
| `idempotency/redis/examples/async-process-order` | Redis idempotency async claim |

---

## 10. Common Patterns and Anti-Patterns

### Patterns

1. **Definition-first is the default** — always start with `DefineLock`, not per-callsite key building
2. **Share definitions** — one `LockDefinition` can have multiple surfaces (Run + Hold + Claim)
3. **Register once at startup** — `Registry.Register(...)` before `lockman.New(...)`
4. **Use `errors.Is`** — always check errors against sentinel errors, never string match
5. **Prefer `BindResourceID`** — unless you genuinely need custom key shapes
6. **Use parent locks over composite** — if the business invariant belongs to one aggregate, don't invent composite
7. **Strict mode only when needed** — adds fencing token overhead; use only for stale writer protection or guarded writes

### Anti-patterns

1. **Building lock keys manually** — defeats the definition-first model
2. **Creating clients per-request** — client is long-lived; register once, reuse
3. **Using `Claim` for sync flows** — `Claim` is for delivery/retry semantics; use `Run` for sync
4. **Using `Run` for async delivery** — `Run` has no idempotency; use `Claim` for message-driven flows
5. **String-matching errors** — always use `errors.Is(err, lockman.ErrXxx)`
6. **Treating inspect data as cluster truth** — inspect is process-local only
7. **Re-entrant acquires** — lockman rejects re-entrant lock acquisition by design
