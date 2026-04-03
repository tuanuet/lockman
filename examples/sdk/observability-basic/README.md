# Observability Basic Example

This example demonstrates wiring observability into the root SDK client.

The example stays on the same definition-first SDK surface as the other `examples/sdk` flows: define a lock boundary first, then attach the execution surface that the client runs.

## What It Shows

- Creating an `observe.Dispatcher` for async event export
- Creating an `inspect.Store` for process-local state
- Wiring both via `lockman.WithObservability(...)`
- Mounting inspection HTTP handlers at `/locks/inspect`
- Running a use case and printing captured events

## Run

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples .
```

## Endpoints

Once running, call:

```bash
curl http://localhost:8080/locks/inspect
curl http://localhost:8080/locks/inspect/events
curl http://localhost:8080/locks/inspect/health
```

## Key Points

- Inspection data is process-local, not cluster truth
- Export failures do not fail the lock lifecycle
- The dispatcher operates on a best-effort basis
