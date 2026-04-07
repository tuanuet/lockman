# PrometheusSink + OTelSink Module Restructuring Design

**Date:** 2026-04-07
**Status:** Draft

## Problem

1. Nhiều team dùng Prometheus trực tiếp thay vì full OTel pipeline. Cần `observe.PrometheusSink`.
2. OTel dependency nằm trong root module — nên tách thành module con riêng biệt.

## Module Layout

```
observe/                    # root observe package (interfaces only)
├── event.go                # Event, EventKind, Sink, Exporter
├── dispatcher.go           # Dispatcher
├── options.go              # Config, Option
├── noop.go                 # NoopSink, NoopExporter, NoopDispatcher
├── otel/                   # module: github.com/tuanuet/lockman/observe/otel
│   ├── go.mod
│   ├── go.sum
│   ├── sink.go             # OTelSink (moved from observe/otel.go)
│   └── sink_test.go        # tests (moved from observe/otel_test.go)
└── prometheus/             # module: github.com/tuanuet/lockman/observe/prometheus
    ├── go.mod
    ├── go.sum
    ├── sink.go             # PrometheusSink (new)
    └── sink_test.go        # tests (new)

examples/sdk/
├── observability-otel/          # OTel example (moved from observability-datadog)
└── observability-prometheus/    # Prometheus example (new)
```

## Import Paths

- Core observe: `github.com/tuanuet/lockman/observe`
- OTelSink: `github.com/tuanuet/lockman/observe/otel`
- PrometheusSink: `github.com/tuanuet/lockman/observe/prometheus`

## Backward Compatibility

Root `observe` package giữ forwarding types/functions với `// Deprecated:` comments:

```go
// Deprecated: use github.com/tuanuet/lockman/observe/otel.OTelConfig
type OTelConfig = otel.OTelConfig

// Deprecated: use github.com/tuanuet/lockman/observe/otel.NewOTelSink
var NewOTelSink = otel.NewOTelSink
```

Điều này cho phép code cũ compile được trong khi migrate dần.

## PrometheusSink Design

### Constructor

```go
func NewPrometheusSink(cfg PrometheusConfig) observe.Sink
```

### PrometheusConfig

```go
type PrometheusConfig struct {
    Registerer prometheus.Registerer // nil = prometheus.DefaultRegisterer
    Namespace  string                // "lockman"
    Subsystem  string                // "" (empty)
}
```

### Metrics Schema

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `lockman_acquire_total` | Counter | `definition_id`, `outcome` | Total lock acquire attempts |
| `lockman_acquire_duration_seconds` | Histogram | `definition_id` | Wait time to acquire |
| `lockman_hold_duration_seconds` | Histogram | `definition_id` | Duration lock was held |
| `lockman_contention_total` | Counter | `definition_id` | Contention events |
| `lockman_active_locks` | Gauge | `definition_id`, `owner_id` | Currently held locks |
| `lockman_renewal_total` | Counter | `definition_id`, `outcome` | Renewal attempts |

**Outcome values:** `"success"`, `"failure"`

### Histogram Buckets

- `acquire_duration_seconds`: `0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10`
- `hold_duration_seconds`: `0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 300`

### Event Mapping

| EventKind | acquire_total | acquire_duration | hold_duration | contention | active_locks | renewal_total |
|-----------|--------------|------------------|---------------|------------|--------------|---------------|
| AcquireSucceeded | +1 (success) | record Wait | | | +1 | |
| AcquireFailed | +1 (failure) | record Wait | | | | |
| Released | | | record Held | | -1 | |
| Contention | | | | +1 | | |
| LeaseLost | | | | | -1 | |
| RenewalSucceeded | | | | | | +1 (success) |
| RenewalFailed | | | | | | +1 (failure) |

**Note:** `AcquireStarted` không được count — chỉ count kết quả (success/failure). Cả OTelSink và PrometheusSink đều dùng semantics này.

### Active Locks Tracking

Internal state required for Gauge:

```go
type activeKey struct {
    definitionID string
    ownerID      string
}

type PrometheusSink struct {
    mu          sync.Mutex
    activeLocks map[activeKey]int
    gaugeVec    *prometheus.GaugeVec
    // ... other collectors
}
```

- Mutex chỉ protect internal map. Gauge operations tự thread-safe.
- Mỗi increment/decrement gọi `gaugeVec.With(labels).Set(float64(count))` — gauge value luôn bằng counter value trong map.
- Khi count = 0, xóa entry khỏi map.
- Dùng `prometheus.GaugeVec` — không cần implement `prometheus.Collector`.

### Consume() Semantics

- Never returns error (best-effort, matches OTelSink)
- Nil registerer → uses `prometheus.DefaultRegisterer`
- Events not in mapping table → ignored

## OTelSink Design (after move)

### Preserved behavior

- `recordSpan()` giữ nguyên — tạo span cho mọi event
- `recordMetrics()` giữ nguyên structure, chỉ update labels
- Constructor `NewOTelSink(cfg OTelConfig) observe.Sink` giữ nguyên signature

### Label changes

OTelSink labels update để match schema mới:
- **Giữ** `lockman.` prefix cho OTel attributes (OTel convention dùng dot-separated names)
- Bỏ `request_id`, `resource_id`, `event_kind`, `contention` attributes
- Thêm `lockman.outcome` attribute (`"success"` / `"failure"`) cho acquire và renewal counters

OTel attributes sau update:
- `lockman.definition_id`
- `lockman.outcome` (cho acquire_total, renewal_total)

## Files Changed

### New
- `observe/otel/go.mod`
- `observe/otel/go.sum`
- `observe/otel/sink.go`
- `observe/otel/sink_test.go`
- `observe/prometheus/go.mod`
- `observe/prometheus/go.sum`
- `observe/prometheus/sink.go`
- `observe/prometheus/sink_test.go`
- `examples/sdk/observability-prometheus/main.go`

### Moved
- `observe/otel.go` → `observe/otel/sink.go`
- `observe/otel_test.go` → `observe/otel/sink_test.go`
- `examples/sdk/observability-datadog/` → `examples/sdk/observability-otel/`

### Modified
- `go.mod` — remove OTel dependencies
- `go.work` — add `./observe/otel`, `./observe/prometheus`
- `observe/event.go` — add deprecated forwarding aliases (OTelConfig, NewOTelSink)
- `examples/go.mod` — add replace directives for `observe/otel` and `observe/prometheus`, add prometheus dependency for new example
- `examples/sdk/observability-datadog/main.go` → `examples/sdk/observability-otel/main.go` — update import paths

### Unchanged
- `internal/observebridge/bridge.go` — chỉ depend trên `observe.Sink` interface, không cần change
- `client.go` — không reference OTelSink trực tiếp
