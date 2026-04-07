# Observability — Datadog Example

This example shows how to sink lockman lifecycle events to Datadog using the
`dd-trace-go` OTel bridge.

## How It Works

1. Start the Datadog tracer (`tracer.Start(...)`)
2. Wrap it as an OpenTelemetry TracerProvider via `opentelemetry.NewTracerProvider()`
3. Pass it to `observe.NewOTelSink(observe.OTelConfig{TracerProvider: ...})`
4. Wire the sink into the dispatcher via `observe.WithSink(otelSink)`

Every lock acquire/release/contention event becomes a Datadog span with
attributes for `definition_id`, `request_id`, `owner_id`, `resource_id`,
`success`, and `contention`.

## Prerequisites

- Datadog Agent running locally (default: `localhost:8126`)
- Or set `DD_AGENT_HOST` to point to a remote agent

## Run

```bash
DD_AGENT_HOST=localhost go run -tags lockman_examples .
```

## Spans You'll See in Datadog

| Span Name | Event |
|---|---|
| `lockman.acquire_started` | Acquire begins |
| `lockman.acquire_succeeded` | Lock acquired |
| `lockman.acquire_failed` | Acquire failed (error recorded) |
| `lockman.released` | Lock released |
| `lockman.contention` | Contention detected |
| `lockman.lease_lost` | Lease lost |
| `lockman.shutdown_completed` | Client shutdown |

## Alternative: OTLP Exporter (No Datadog Dependency)

If you prefer not to depend on `dd-trace-go`, use standard OTel OTLP exporters
pointing at Datadog's OTLP endpoint. See the OTel SDK docs for setup.
