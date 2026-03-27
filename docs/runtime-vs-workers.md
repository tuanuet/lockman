# Runtime vs Workers

This document explains when to use `runtime` and when to use `workers`.

At a high level, both packages coordinate access to shared resources with the same registry and driver model. The real difference is not "one has locks and the other does not". The difference is the execution contract they are built for.

## Short Version

- Use `runtime` for synchronous application flows.
- Use `workers` for asynchronous message-processing flows.

If your code is running inside a direct request or command and the caller is waiting for the result now, start with `runtime`.

If your code is running because a queue delivery arrived and you must decide whether to ack, retry, drop, or dead-letter the message, use `workers`.

## Why `runtime` Exists

`runtime` exists to protect a synchronous critical section.

The caller already owns the outer execution lifecycle:

- request enters the service
- app decides whether to wait or fail fast
- business code runs
- caller receives the result directly

In that model, the lock layer should stay focused on coordination:

- acquire
- execute
- release
- optionally inspect advisory presence

`runtime` is therefore the right fit when lock coordination is part of a normal application call path rather than a message-delivery lifecycle.

## Why `workers` Exists

`workers` exists because async consumers have a different problem shape.

A queue consumer is not just protecting a critical section. It is also handling delivery semantics:

- duplicate delivery
- retry decisions
- lease renewal while processing
- shutdown draining
- normalized outcomes for queue adapters

That is why `workers` adds idempotency and outcome mapping on top of lock execution. In practice, `workers` is "coordination for message handling", not just "coordination for code execution".

## When To Use `runtime`

Use `runtime` when:

- the flow is triggered synchronously by HTTP, RPC, CLI, or an internal service call
- the caller expects an immediate result
- duplicate delivery semantics are not part of the problem
- the application layer wants to handle returned errors directly
- lock ownership begins and ends inside one direct code path

Typical examples:

- `POST /orders/{id}/approve`
- an admin command that recalculates one aggregate
- a synchronous inventory reservation endpoint
- a transfer API that must lock two accounts in one request

## When To Use `workers`

Use `workers` when:

- the flow starts from a queue or event delivery
- the same message may be delivered more than once
- the consumer must decide whether to ack, retry, drop, or dead-letter
- the handler may run long enough to require SDK-managed lease renewal
- graceful shutdown must stop new claims and drain in-flight handlers safely

Typical examples:

- Kafka consumer handling `order.approve_requested`
- SQS worker processing payment reconciliation jobs
- RabbitMQ consumer executing transfer requests
- background job processor handling idempotent replays

## Benefits of `runtime`

`runtime` is simpler when async delivery semantics are irrelevant.

Benefits:

- smaller mental model
- direct error handling
- no idempotency store requirement
- good fit for request/response systems
- easier to embed in synchronous application services

In short, `runtime` keeps the lock layer narrow when all you need is protected execution.

## Benefits of `workers`

`workers` is valuable when lock coordination alone is not enough.

Benefits:

- absorbs duplicate deliveries through idempotency
- normalizes handler outcomes for queue adapters
- renews leases while handlers are still running
- protects shutdown behavior for in-flight message handlers
- keeps message-processing policy consistent across consumers

In short, `workers` gives you one place to combine distributed locking and async-delivery safety.

## Same Lock, Different Execution Contract

It is normal for the two packages to look similar at first glance.

Both can:

- acquire a lock
- run a callback
- release the lock

That shared shape is intentional. The difference is what surrounds that execution.

`runtime` says:

- "run this protected section for my synchronous caller"

`workers` says:

- "run this protected section as part of a message-delivery lifecycle"

That difference changes what the SDK must manage before and after the callback.

## Concrete Example: Approve Order

### `runtime`

An HTTP handler receives `POST /orders/123/approve`.

The service uses `runtime.ExecuteExclusive(...)` to:

- lock `order:123`
- validate current state
- write the approval
- release the lock
- return success or failure directly to the caller

The system does not need message idempotency here because the request path itself owns retry behavior.

### `workers`

A Kafka consumer receives `order.approve_requested` for `order:123`.

The service uses `workers.ExecuteClaimed(...)` to:

- begin idempotency for the message
- lock `order:123`
- run the approval handler
- persist terminal idempotency state
- release the lock
- map the result into ack, retry, drop, or dlq behavior in the queue adapter

The business action may look similar, but the outer contract is different because the consumer must handle duplicate delivery and retry semantics safely.

## Concrete Example: Transfer Between Two Accounts

### `runtime`

A direct API request starts a transfer from account `A` to account `B`.

Use `runtime.ExecuteCompositeExclusive(...)` when the transfer is a synchronous request/response operation and the caller is waiting for the result now.

### `workers`

A queue message `transfer.requested` asks a consumer to move funds from account `A` to account `B`.

Use `workers.ExecuteCompositeClaimed(...)` when the transfer is handled asynchronously and the message may be retried or redelivered.

## Decision Table

| Question | Prefer `runtime` | Prefer `workers` |
|---|---|---|
| What triggered the flow? | Direct caller | Queue delivery |
| Who owns retry semantics? | Caller / application layer | Queue adapter / consumer layer |
| Is duplicate delivery a first-class concern? | No | Yes |
| Do you need idempotency state? | Usually no | Usually yes |
| Do you need ack/retry/drop/dlq semantics? | No | Yes |
| Is the flow request/response oriented? | Yes | No |

## Rule of Thumb

- If the user or caller is waiting on this exact execution, use `runtime`.
- If the system is processing a delivery that may be replayed later, use `workers`.

## Final Guidance

Do not choose `workers` just because the code runs in a goroutine.

Do not choose `runtime` just because the handler body is simple.

Choose based on the lifecycle you are integrating with:

- synchronous call lifecycle -> `runtime`
- asynchronous delivery lifecycle -> `workers`
