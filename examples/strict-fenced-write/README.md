# Strict Fenced Write Example

This example source is kept in the root workspace for discoverability. The root `main.go` is gated behind the `lockman_examples` build tag so default root verification stays clean.

Run the preserved root copy from the workspace root:

```bash
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run -tags lockman_examples ./examples/strict-fenced-write
```

Canonical published runnable path:

```bash
cd redis
LOCKMAN_REDIS_URL=redis://localhost:6379/0 go run ./examples/strict-fenced-write
```
