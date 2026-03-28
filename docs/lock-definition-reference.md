# Use Case Definition Reference

The public SDK starts from use cases, not raw lock definitions.

## Sync Use Cases

Use `lockman.DefineRun[T](...)` for synchronous critical sections:

```go
type ApproveInput struct {
	OrderID string
}

var Approve = lockman.DefineRun[ApproveInput](
	"order.approve",
	lockman.BindResourceID("order", func(in ApproveInput) string { return in.OrderID }),
	lockman.TTL(30*time.Second),
)
```

## Async Use Cases

Use `lockman.DefineClaim[T](...)` when the flow starts from message delivery:

```go
type ProcessInput struct {
	OrderID string
}

var Process = lockman.DefineClaim[ProcessInput](
	"order.process",
	lockman.BindResourceID("order", func(in ProcessInput) string { return in.OrderID }),
	lockman.TTL(30*time.Second),
	lockman.Idempotent(),
)
```

Register both sync and async use cases centrally before creating the client:

```go
reg := lockman.NewRegistry()
if err := reg.Register(Approve, Process); err != nil {
	return err
}
```

## Binding Helpers

- `lockman.BindResourceID("resource", fn)`: preferred happy path for single-resource use cases
- `lockman.BindKey(fn)`: use only when the resource key shape is genuinely custom

## Core Options

- `lockman.TTL(...)`: lease TTL hint
- `lockman.WaitTimeout(...)`: acquire wait budget
- `lockman.Idempotent()`: required for claim use cases that must deduplicate deliveries

## Advanced Definition Surfaces

The default path is still root `lockman`, but some advanced cases live in explicit packages:

- strict fenced runs: [`docs/advanced/strict.md`](advanced/strict.md)
- composite sync runs: [`docs/advanced/composite.md`](advanced/composite.md)

If you need lower-level authoring primitives than typed use cases and bindings, you are outside the default SDK layer.
