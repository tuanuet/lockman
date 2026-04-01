# Run vs Claim

In the public SDK, you choose between two entry points:

- `Run`: protect a synchronous critical section
- `Claim`: protect asynchronous message processing

That is the first lifecycle decision a new user makes. Everything else builds on top of it.

## Use `Run`

Choose `Run` when:

- a caller is waiting for the result now
- the flow is HTTP, RPC, CLI, or an internal direct call
- duplicate delivery is not a first-class concern
- you want one lock, one callback, one returned error

Typical examples:

- approve one order
- reserve inventory during a request
- execute a composite transfer inside an API call
- perform a strict fenced write in a synchronous command

## Use `Claim`

Choose `Claim` when:

- the flow starts from a queue or delivery
- duplicate delivery must be absorbed safely
- you need idempotency state
- retry and shutdown behavior matter as much as the callback itself

Typical examples:

- process an order event
- consume a reconciliation job
- run a background worker that may see redeliveries

## The Mental Model

`Run` says:

- bind typed input
- acquire the lock
- execute the callback
- release the lock

`Claim` says:

- bind typed input plus delivery metadata
- begin idempotency
- acquire the lock
- execute the callback
- persist terminal idempotency state
- release the lock

## Decision Table

| Question | Choose `Run` | Choose `Claim` |
|---|---|---|
| What triggered the work? | Direct caller | Message delivery |
| Is duplicate delivery a built-in concern? | No | Yes |
| Do you need an idempotency store? | Usually no | Usually yes |
| Who owns retry semantics? | Caller / application code | Queue adapter / worker code |
| What do you pass to the use case? | Typed input | Typed input + `lockman.Delivery` |

## Examples

- Sync: [`docs/quickstart-sync.md`](quickstart-sync.md)
- Async: [`docs/quickstart-async.md`](quickstart-async.md)
- Composite sync: [`docs/advanced/composite.md`](advanced/composite.md)
- Strict sync: [`docs/advanced/strict.md`](advanced/strict.md)
- Observability SDK: [`examples/sdk/observability-basic`](../examples/sdk/observability-basic)
- Observability core: [`examples/core/observability-runtime`](../examples/core/observability-runtime)

## Observability Applies To Both Paths

The `observe` and `inspect` packages work with both `Run` and `Claim` paths. Whether you use the root SDK or direct engine wiring, observability events are emitted for acquire, release, renewal, and shutdown lifecycle.
