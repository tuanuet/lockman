# Registry And Use Cases

The registry is mandatory, but it should stay a startup concern rather than leak into every callsite.

## Define In Code

Define typed use cases next to the domain code that owns them.

```go
var Approve = lockman.DefineRun[ApproveInput](...)
var Process = lockman.DefineClaim[ProcessInput](...)
```

## Register Centrally

Collect them in one place at startup:

```go
reg := lockman.NewRegistry()
if err := reg.Register(
	orderlocks.Approve,
	orderlocks.Process,
	paymentlocks.Capture,
); err != nil {
	return err
}
```

## Why The Registry Still Matters

The registry is where `lockman` records the allowed use case set and enforces registration invariants:

- duplicate public name rejection
- registry ownership and mismatch detection

Client construction then validates that full registered set against the configured backend and idempotency capabilities.

## What Callsites Should See

Callsites should see:

- typed input
- one `With(...)`
- one `Run(...)` or `Claim(...)`

Callsites should not need to pass raw definition IDs, registry lookups, or `map[string]string`.

Runnable examples that need concrete adapters still live with those adapter modules. The registry contract stays in the root SDK, the workspace SDK mirrors live under `examples/sdk/...`, and the lower-level scenario examples live under `examples/core/...`.
