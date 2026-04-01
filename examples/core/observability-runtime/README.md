# Observability Runtime Example

This example demonstrates using the `observe.Dispatcher` directly for event export.

## What It Shows

- Creating an `observe.Dispatcher` with a custom exporter
- Publishing events directly to the dispatcher
- Best-effort async event delivery

## Run

```bash
go run -tags lockman_examples .
```

## Key Points

- The dispatcher is non-blocking - `Publish` returns immediately
- Export failures are best-effort and don't affect the caller
- The dispatcher can be used independently of the root SDK
