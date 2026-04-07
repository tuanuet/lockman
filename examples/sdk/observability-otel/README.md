# Observability — OTel Example

This example shows how to sink lockman lifecycle events to any OpenTelemetry-compatible
backend using the `otel` package.

## How It Works

1. Start an OTel-compatible tracer (Datadog, Jaeger, Zipkin, etc.)
2. Pass its TracerProvider to `otel.NewOTelSink(otel.OTelConfig{TracerProvider: ...})`
3. Wire the sink into the dispatcher via `observe.WithSink(otelSink)`

Every lock acquire/release/contention event becomes an OTel span with
attributes for `definition_id`, `request_id`, `owner_id`, `resource_id`,
`success`, and `contention`.

## Prerequisites

- An OTel-compatible tracer running locally or remotely
- For Datadog: set `DD_AGENT_HOST` to point to the agent

## Run

```bash
DD_AGENT_HOST=localhost go run -tags lockman_examples .
```

## Spans You'll See in Your OTel Backend

| Span Name | Event |
|---|---|
| `lockman.acquire_started` | Acquire begins |
| `lockman.acquire_succeeded` | Lock acquired |
| `lockman.acquire_failed` | Acquire failed (error recorded) |
| `lockman.released` | Lock released |
| `lockman.contention` | Contention detected |
| `lockman.lease_lost` | Lease lost |
| `lockman.shutdown_completed` | Client shutdown |
