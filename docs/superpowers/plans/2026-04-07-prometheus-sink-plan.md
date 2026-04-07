# PrometheusSink + OTelSink Module Restructuring Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create PrometheusSink as a standalone module and move OTelSink into its own module, removing OTel/Prometheus dependencies from the root module.

**Architecture:** Two new submodules (`observe/otel`, `observe/prometheus`) each implement the `observe.Sink` interface. Root `observe` package keeps deprecated forwarding aliases for backward compatibility. Examples move to `examples/sdk/`.

**Tech Stack:** Go 1.24+, `github.com/prometheus/client_golang/prometheus`, `go.opentelemetry.io/otel`, Go workspace (`go.work`)

---

## Chunk 1: Move OTelSink to observe/otel module

### Task 1: Create observe/otel module with OTelSink

**Files:**
- Create: `observe/otel/go.mod`
- Create: `observe/otel/sink.go` (moved from `observe/otel.go`)
- Create: `observe/otel/sink_test.go` (moved from `observe/otel_test.go`)
- Modify: `go.work` — add `./observe/otel`
- Modify: `go.mod` — remove OTel dependencies

- [ ] **Step 1: Create observe/otel directory**

```bash
mkdir -p observe/otel
```

- [ ] **Step 2: Create observe/otel/go.mod**

```go
module github.com/tuanuet/lockman/observe/otel

go 1.24

require (
	github.com/tuanuet/lockman v0.0.0-00010101000000-000000000000
	go.opentelemetry.io/otel v1.35.0
	go.opentelemetry.io/otel/metric v1.35.0
	go.opentelemetry.io/otel/trace v1.35.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	go.opentelemetry.io/otel/sdk v1.35.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.35.0 // indirect
)

replace github.com/tuanuet/lockman => ../..
```

- [ ] **Step 3: Read observe/otel.go and create observe/otel/sink.go**

Read `observe/otel.go` content. Create `observe/otel/sink.go` with:
- Package declaration: `package otel`
- Import `github.com/tuanuet/lockman/observe`
- Keep all types, functions, and logic identical to current `observe/otel.go`
- Update `NewOTelSink` return type to `observe.Sink`
- Update internal references from `Event` → `observe.Event`, `EventAcquireStarted` → `observe.EventAcquireStarted`, etc.
- Update `commonAttributes` to use new label schema:
  - Keep `lockman.` prefix (OTel convention)
  - Labels: `lockman.definition_id`, `lockman.outcome` (for acquire/renewal counters)
  - Remove: `lockman.request_id`, `lockman.resource_id`, `lockman.event_kind`, `lockman.success`, `lockman.contention`

- [ ] **Step 4: Read observe/otel_test.go and create observe/otel/sink_test.go**

Read `observe/otel_test.go` content. Create `observe/otel/sink_test.go` with:
- Package declaration: `package otel`
- Import `github.com/tuanuet/lockman/observe`
- Update all references to use `observe.Event`, `observe.EventKind`, etc.
- Update test `TestOTelSinkSatisfiesSinkInterface` to verify `observe.Sink` interface

- [ ] **Step 5: Update go.work**

Read `go.work`. Add:
```
	./observe/otel
```

- [ ] **Step 6: Remove OTel deps from root go.mod**

Read `go.mod`. Remove:
```
	go.opentelemetry.io/otel v1.35.0
	go.opentelemetry.io/otel/metric v1.35.0
	go.opentelemetry.io/otel/trace v1.35.0
```

Keep only the module declaration and test dependencies.

- [ ] **Step 7: Run go mod tidy in observe/otel**

```bash
cd observe/otel && go mod tidy
```

- [ ] **Step 8: Run OTel tests**

```bash
go test ./observe/otel/... -v
```

Expected: All tests pass.

- [ ] **Step 9: Delete old files**

```bash
rm observe/otel.go observe/otel_test.go
```

- [ ] **Step 10: Run root tests to verify no breakage**

```bash
go test ./... -run '^$'
```

Expected: Compiles successfully.

- [ ] **Step 11: Commit**

```bash
git add observe/otel/ go.mod go.work
git rm observe/otel.go observe/otel_test.go
git commit -m "refactor: move OTelSink to observe/otel submodule"
```

---

## Chunk 2: Create observe/prometheus module

### Task 2: Create observe/prometheus module with PrometheusSink

**Files:**
- Create: `observe/prometheus/go.mod`
- Create: `observe/prometheus/sink.go`
- Create: `observe/prometheus/sink_test.go`
- Modify: `go.work` — add `./observe/prometheus`

- [ ] **Step 1: Create observe/prometheus directory**

```bash
mkdir -p observe/prometheus
```

- [ ] **Step 2: Create observe/prometheus/go.mod**

```go
module github.com/tuanuet/lockman/observe/prometheus

go 1.24

require (
	github.com/prometheus/client_golang v1.20.5
	github.com/tuanuet/lockman v0.0.0-00010101000000-000000000000
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.60.1 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	golang.org/x/sys v0.26.0 // indirect
	google.golang.org/protobuf v1.35.1 // indirect
)

replace github.com/tuanuet/lockman => ../..
```

- [ ] **Step 3: Create observe/prometheus/sink.go**

```go
package prometheus

import (
	"context"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/tuanuet/lockman/observe"
)

// PrometheusConfig holds Prometheus registry settings for the PrometheusSink.
// If Registerer is nil, prometheus.DefaultRegisterer is used.
type PrometheusConfig struct {
	Registerer prometheus.Registerer
	Namespace  string
	Subsystem  string
}

// PrometheusSink implements observe.Sink by recording lock lifecycle events
// as Prometheus metrics.
type PrometheusSink struct {
	acquireTotal    *prometheus.CounterVec
	acquireDuration *prometheus.HistogramVec
	holdDuration    *prometheus.HistogramVec
	contentionTotal *prometheus.CounterVec
	activeLocks     *prometheus.GaugeVec
	renewalTotal    *prometheus.CounterVec

	mu          sync.Mutex
	activeLocksMap map[activeKey]int
}

type activeKey struct {
	definitionID string
	ownerID      string
}

// NewPrometheusSink creates a Sink that records lock lifecycle events as
// Prometheus metrics. Pass nil Registerer to use prometheus.DefaultRegisterer.
// The returned sink never returns errors from Consume.
func NewPrometheusSink(cfg PrometheusConfig) observe.Sink {
	if cfg.Registerer == nil {
		cfg.Registerer = prometheus.DefaultRegisterer
	}

	ns := cfg.Namespace
	ss := cfg.Subsystem

	s := &PrometheusSink{
		activeLocksMap: make(map[activeKey]int),
	}

	s.acquireTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   ns,
		Subsystem:   ss,
		Name:        "acquire_total",
		Help:        "Total number of lock acquire attempts",
	}, []string{"definition_id", "outcome"})

	s.acquireDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: ns,
		Subsystem: ss,
		Name:      "acquire_duration_seconds",
		Help:      "Time spent waiting to acquire a lock",
		Buckets:   []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"definition_id"})

	s.holdDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: ns,
		Subsystem: ss,
		Name:      "hold_duration_seconds",
		Help:      "Duration a lock was held",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300},
	}, []string{"definition_id"})

	s.contentionTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   ns,
		Subsystem:   ss,
		Name:        "contention_total",
		Help:        "Number of lock contention events",
	}, []string{"definition_id"})

	s.activeLocks = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: ns,
		Subsystem: ss,
		Name:      "active_locks",
		Help:      "Number of currently held locks",
	}, []string{"definition_id", "owner_id"})

	s.renewalTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   ns,
		Subsystem:   ss,
		Name:        "renewal_total",
		Help:        "Total number of lock renewal attempts",
	}, []string{"definition_id", "outcome"})

	collectors := []prometheus.Collector{
		s.acquireTotal,
		s.acquireDuration,
		s.holdDuration,
		s.contentionTotal,
		s.activeLocks,
		s.renewalTotal,
	}

	for _, c := range collectors {
		cfg.Registerer.MustRegister(c)
	}

	return s
}

// Consume records the event as Prometheus metrics.
// It never returns an error (best-effort semantics).
func (s *PrometheusSink) Consume(_ context.Context, event observe.Event) error {
	s.recordMetrics(event)
	return nil
}

func (s *PrometheusSink) recordMetrics(event observe.Event) {
	labels := prometheus.Labels{"definition_id": event.DefinitionID}

	switch event.Kind {
	case observe.EventAcquireSucceeded:
		s.acquireTotal.With(prometheus.Labels{
			"definition_id": event.DefinitionID,
			"outcome":       "success",
		}).Inc()
		if event.Wait > 0 {
			s.acquireDuration.With(labels).Observe(event.Wait.Seconds())
		}
		s.mu.Lock()
		key := activeKey{definitionID: event.DefinitionID, ownerID: event.OwnerID}
		s.activeLocksMap[key]++
		s.activeLocks.With(prometheus.Labels{
			"definition_id": event.DefinitionID,
			"owner_id":      event.OwnerID,
		}).Set(float64(s.activeLocksMap[key]))
		s.mu.Unlock()

	case observe.EventAcquireFailed:
		s.acquireTotal.With(prometheus.Labels{
			"definition_id": event.DefinitionID,
			"outcome":       "failure",
		}).Inc()
		if event.Wait > 0 {
			s.acquireDuration.With(labels).Observe(event.Wait.Seconds())
		}

	case observe.EventReleased:
		if event.Held > 0 {
			s.holdDuration.With(labels).Observe(event.Held.Seconds())
		}
		s.mu.Lock()
		key := activeKey{definitionID: event.DefinitionID, ownerID: event.OwnerID}
		if count, ok := s.activeLocksMap[key]; ok {
			count--
			if count <= 0 {
				delete(s.activeLocksMap, key)
			} else {
				s.activeLocksMap[key] = count
			}
			s.activeLocks.With(prometheus.Labels{
				"definition_id": event.DefinitionID,
				"owner_id":      event.OwnerID,
			}).Set(float64(count))
		}
		s.mu.Unlock()

	case observe.EventContention:
		s.contentionTotal.With(labels).Inc()

	case observe.EventLeaseLost:
		s.mu.Lock()
		key := activeKey{definitionID: event.DefinitionID, ownerID: event.OwnerID}
		if count, ok := s.activeLocksMap[key]; ok {
			count--
			if count <= 0 {
				delete(s.activeLocksMap, key)
			} else {
				s.activeLocksMap[key] = count
			}
			s.activeLocks.With(prometheus.Labels{
				"definition_id": event.DefinitionID,
				"owner_id":      event.OwnerID,
			}).Set(float64(count))
		}
		s.mu.Unlock()

	case observe.EventRenewalSucceeded:
		s.renewalTotal.With(prometheus.Labels{
			"definition_id": event.DefinitionID,
			"outcome":       "success",
		}).Inc()

	case observe.EventRenewalFailed:
		s.renewalTotal.With(prometheus.Labels{
			"definition_id": event.DefinitionID,
			"outcome":       "failure",
		}).Inc()
	}
}

var _ observe.Sink = (*PrometheusSink)(nil)
```

- [ ] **Step 4: Create observe/prometheus/sink_test.go**

Tests to write:
1. `TestPrometheusSinkSatisfiesSinkInterface` — verify implements `observe.Sink`
2. `TestPrometheusSinkConsumeNeverReturnsError` — Consume always returns nil
3. `TestPrometheusSinkConsumeAcquireSucceeded` — acquire_total +1, active_locks +1, acquire_duration recorded
4. `TestPrometheusSinkConsumeAcquireFailed` — acquire_total +1 failure, no active_locks change
5. `TestPrometheusSinkConsumeReleased` — hold_duration recorded, active_locks -1
6. `TestPrometheusSinkConsumeContention` — contention_total +1
7. `TestPrometheusSinkConsumeLeaseLost` — active_locks -1
8. `TestPrometheusSinkConsumeRenewalSucceeded` — renewal_total +1 success
9. `TestPrometheusSinkConsumeRenewalFailed` — renewal_total +1 failure
10. `TestPrometheusSinkIgnoresUnknownEvents` — unmapped events don't affect metrics
11. `TestPrometheusSinkActiveLocksMultipleOwners` — gauge tracks per owner independently
12. `TestPrometheusSinkActiveLocksCleansUpZeroCount` — removes entry when count reaches 0

Use `prometheus.NewRegistry()` for test isolation. Read metrics via `registry.Gather()`.

- [ ] **Step 5: Update go.work**

Add `./observe/prometheus` to the use block.

- [ ] **Step 6: Run go mod tidy**

```bash
cd observe/prometheus && go mod tidy
```

- [ ] **Step 7: Run Prometheus tests**

```bash
go test ./observe/prometheus/... -v
```

Expected: All tests pass.

- [ ] **Step 8: Commit**

```bash
git add observe/prometheus/ go.work
git commit -m "feat: add PrometheusSink module"
```

---

## Chunk 3: Backward compatibility + examples

### Task 3: Add deprecated aliases and update examples

**Files:**
- Modify: `observe/event.go` — add deprecated forwarding aliases
- Create: `examples/sdk/observability-otel/main.go` (moved from observability-datadog)
- Create: `examples/sdk/observability-otel/main_test.go`
- Create: `examples/sdk/observability-otel/README.md`
- Create: `examples/sdk/observability-prometheus/main.go`
- Modify: `examples/go.mod` — add replace directives and prometheus dependency

- [ ] **Step 1: Add deprecated aliases to observe/event.go**

Append to `observe/event.go`:

```go
// Deprecated: use github.com/tuanuet/lockman/observe/otel.OTelConfig
// This alias exists for backward compatibility only.
type OTelConfig = otel.OTelConfig

// Deprecated: use github.com/tuanuet/lockman/observe/otel.NewOTelSink
// This alias exists for backward compatibility only.
var NewOTelSink = otel.NewOTelSink
```

Add import: `"github.com/tuanuet/lockman/observe/otel"` to root observe package.

Wait — this creates a circular dependency. Root `observe` cannot import `observe/otel` because `observe/otel` imports `observe`.

**Correct approach:** Create `observe/otel_alias.go` as a separate file with build tag or just document the breaking change. Actually, the cleanest approach is to NOT provide aliases and just update consumers. Let me revise:

Skip the alias approach entirely. Instead:
- Update `examples/go.mod` with new replace directives
- Move example to new path with updated imports
- Add a note in CHANGELOG about the import path change

- [ ] **Step 1 (revised): Create examples/sdk/observability-otel from observability-datadog**

Read `examples/sdk/observability-datadog/main.go`. Create `examples/sdk/observability-otel/main.go` with:
- Updated import: `github.com/tuanuet/lockman/observe/otel` instead of `observe.NewOTelSink`
- Rename references from Datadog-specific naming to generic OTel

Copy `main_test.go` and `README.md`, update imports and references.

- [ ] **Step 2: Create examples/sdk/observability-prometheus/main.go**

```go
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/observe"
	"github.com/tuanuet/lockman/observe/prometheus"
)

var orderDef = lockman.DefineLock(
	"order",
	lockman.BindResourceID("order", func(in OrderInput) string { return in.OrderID }),
)

var approve = lockman.DefineRunOn("order.approve", orderDef)

type OrderInput struct {
	OrderID string
}

func main() {
	reg := lockman.NewRegistry()
	if err := reg.Register(approve); err != nil {
		log.Fatal(err)
	}

	// Create Prometheus sink
	promSink := prometheus.NewPrometheusSink(prometheus.PrometheusConfig{
		Namespace: "lockman",
	})

	// Create dispatcher
	dispatcher := observe.NewDispatcher(
		observe.WithSink(promSink),
		observe.WithBufferSize(1024),
	)
	defer dispatcher.Shutdown(context.Background())

	// Create client
	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithBackend(backend), // configure your backend
		lockman.WithObserver(dispatcher),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Shutdown(context.Background())

	// Expose /metrics endpoint
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Println("metrics available at http://localhost:9090/metrics")
		log.Fatal(http.ListenAndServe(":9090", nil))
	}()

	// Execute a lock
	req, _ := approve.With(OrderInput{OrderID: "123"})
	if err := client.Run(context.Background(), req, func(ctx context.Context, lease lockman.Lease) error {
		fmt.Println("processing order:", lease.ResourceKey)
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	// Keep running to serve metrics
	select {}
}
```

- [ ] **Step 3: Update examples/go.mod**

Read `examples/go.mod`. Add:

```go
require (
	// ... existing ...
	github.com/prometheus/client_golang v1.20.5
	github.com/tuanuet/lockman/observe/otel v0.0.0-00010101000000-000000000000
	github.com/tuanuet/lockman/observe/prometheus v0.0.0-00010101000000-000000000000
)

// ... existing replace directives ...

replace github.com/tuanuet/lockman/observe/otel => ../observe/otel

replace github.com/tuanuet/lockman/observe/prometheus => ../observe/prometheus
```

- [ ] **Step 4: Remove old observability-datadog example**

```bash
rm -rf examples/sdk/observability-datadog
```

- [ ] **Step 5: Run go mod tidy in examples**

```bash
cd examples && go mod tidy
```

- [ ] **Step 6: Compile examples**

```bash
go test -tags lockman_examples ./examples/... -run '^$'
```

Expected: Compiles successfully.

- [ ] **Step 7: Commit**

```bash
git add examples/sdk/observability-otel/ examples/sdk/observability-prometheus/ examples/go.mod
git rm -rf examples/sdk/observability-datadog
git commit -m "refactor: update examples for new observe module structure"
```

---

## Chunk 4: OTelSink metrics label update + final verification

### Task 4: Update OTelSink labels to match new schema

**Files:**
- Modify: `observe/otel/sink.go` — update labels in commonAttributes and recordMetrics

- [ ] **Step 1: Update OTelSink labels**

In `observe/otel/sink.go`:

Update `commonAttributes` to:
```go
func (s *OTelSink) commonAttributes(event observe.Event) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("lockman.definition_id", event.DefinitionID),
	}
	return attrs
}
```

Update `recordMetrics` to use `outcome` attribute:
```go
func (s *OTelSink) recordMetrics(ctx context.Context, event observe.Event) {
	if s.meter == nil {
		return
	}

	switch event.Kind {
	case observe.EventAcquireSucceeded, observe.EventAcquireFailed:
		outcome := "success"
		if event.Kind == observe.EventAcquireFailed {
			outcome = "failure"
		}
		attrs := append(s.commonAttributes(event), attribute.String("lockman.outcome", outcome))
		s.acquireTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
		if event.Wait > 0 {
			s.acquireDuration.Record(ctx, event.Wait.Seconds(), metric.WithAttributes(s.commonAttributes(event)...))
		}
	case observe.EventContention:
		s.contentionCount.Add(ctx, 1, metric.WithAttributes(s.commonAttributes(event)...))
	case observe.EventReleased:
		if event.Held > 0 {
			s.holdDuration.Record(ctx, event.Held.Seconds(), metric.WithAttributes(s.commonAttributes(event)...))
		}
	case observe.EventRenewalSucceeded, observe.EventRenewalFailed:
		outcome := "success"
		if event.Kind == observe.EventRenewalFailed {
			outcome = "failure"
		}
		attrs := append(s.commonAttributes(event), attribute.String("lockman.outcome", outcome))
		s.renewalTotal.Add(ctx, 1, metric.WithAttributes(attrs...))
	}
}
```

Update `initMetrics` to add `renewalTotal`:
```go
func (s *OTelSink) initMetrics() {
	s.acquireTotal, _ = s.meter.Int64Counter(
		"lockman.acquire.total",
		metric.WithDescription("Total number of lock acquire attempts"),
	)
	s.acquireDuration, _ = s.meter.Float64Histogram(
		"lockman.acquire.duration",
		metric.WithDescription("Time spent waiting to acquire a lock in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	s.contentionCount, _ = s.meter.Int64Counter(
		"lockman.contentention.count",
		metric.WithDescription("Number of lock contention events"),
	)
	s.holdDuration, _ = s.meter.Float64Histogram(
		"lockman.hold.duration",
		metric.WithDescription("Duration a lock was held in seconds"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300),
	)
	s.renewalTotal, _ = s.meter.Int64Counter(
		"lockman.renewal.total",
		metric.WithDescription("Total number of lock renewal attempts"),
	)
}
```

Update `OTelSink` struct to add `renewalTotal`:
```go
type OTelSink struct {
	tracer trace.Tracer
	meter  metric.Meter

	acquireTotal    metric.Int64Counter
	acquireDuration metric.Float64Histogram
	contentionCount metric.Int64Counter
	holdDuration    metric.Float64Histogram
	renewalTotal    metric.Int64Counter
}
```

- [ ] **Step 2: Update OTelSink tests**

Update tests in `observe/otel/sink_test.go` to reflect new label schema. Tests that check attributes should use the new label set.

- [ ] **Step 3: Run OTel tests**

```bash
go test ./observe/otel/... -v
```

Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add observe/otel/
git commit -m "refactor: update OTelSink metrics to match new schema"
```

---

## Chunk 5: Final verification

### Task 5: Run full test suite and CI parity checks

- [ ] **Step 1: Run full test suite**

```bash
go test ./...
```

- [ ] **Step 2: Run tests without workspace mode**

```bash
GOWORK=off go test ./...
```

- [ ] **Step 3: Run module-specific tests**

```bash
go test ./observe/otel/...
go test ./observe/prometheus/...
go test ./backend/redis/...
go test ./idempotency/redis/...
go test ./guard/postgres/...
```

- [ ] **Step 4: Compile examples**

```bash
go test -tags lockman_examples ./examples/... -run '^$'
```

- [ ] **Step 5: Run lint**

```bash
make lint
```

- [ ] **Step 6: Run tidy**

```bash
make tidy
```

- [ ] **Step 7: Run benchmarks compile check**

```bash
go test -run '^$' ./benchmarks
```

Expected: All pass.
