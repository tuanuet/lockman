# Use Case Definition Reference

From `v1.3.0`, the public SDK starts from lock definitions first, then attaches typed execution surfaces to those definitions.

## Shared Lock Definitions

Use `lockman.DefineLock(...)` when multiple public use cases should share one lock identity:

```go
type OrderInput struct {
	OrderID string
}

var OrderDef = lockman.DefineLock(
	"order",
	lockman.BindResourceID("order", func(in OrderInput) string { return in.OrderID }),
)

var Approve = lockman.DefineRunOn("order.approve", OrderDef)
var Process = lockman.DefineClaimOn("order.process", OrderDef, lockman.Idempotent())
```

## Sync Use Cases

Use `lockman.DefineRunOn[T](...)` to attach a synchronous execution surface to an existing lock definition:

```go
var Approve = lockman.DefineRunOn("order.approve", OrderDef, lockman.TTL(30*time.Second))
```

## Async Use Cases

Use `lockman.DefineClaimOn[T](...)` when the flow starts from message delivery and should attach to an existing lock definition:

```go
var Process = lockman.DefineClaimOn("order.process", OrderDef, lockman.TTL(30*time.Second), lockman.Idempotent())
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

## Hold Use Cases

Use `lockman.DefineHoldOn[T](...)` to attach a hold execution surface to an existing lock definition:

```go
var ManualHold = lockman.DefineHoldOn("order.manual_hold", OrderDef)
```

## Canonical Example

Start with [`examples/sdk/shared-lock-definition`](../examples/sdk/shared-lock-definition) when you want the smallest runnable example of the `v1.3.0` definition-first SDK model.

## Advanced Definition Surfaces

The default path is still root `github.com/tuanuet/lockman`, but some advanced cases live in explicit packages:

- strict fenced runs: [`docs/advanced/strict.md`](advanced/strict.md)
- composite sync runs: [`docs/advanced/composite.md`](advanced/composite.md)

If you need lower-level authoring primitives than typed use cases and bindings, you are outside the default SDK layer.
