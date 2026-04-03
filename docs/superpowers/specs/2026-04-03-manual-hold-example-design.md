# Dedicated Hold Example Design

## Goal

Add one dedicated SDK example for the `Hold` execution surface so readers can learn the acquire-and-forfeit flow without having to infer it from the shared-definition example.

## Scope

- add `examples/sdk/manual-hold/main.go`
- keep the example focused on `DefineLock`, `DefineHoldOn`, `client.Hold`, and `client.Forfeit`
- print the resource key and opaque hold token
- update docs that enumerate the learning-path examples so the new example is discoverable

## Non-Goals

- do not add `Run` or `Claim` behavior into this example
- do not demonstrate strict or composite behavior
- do not add a second adapter-specific example in this change

## Example Shape

The example should match the existing SDK example structure:

1. define a shared lock definition for one business resource
2. attach one hold use case with `DefineHoldOn`
3. register the use case in a root registry
4. construct a root client with Redis backend
5. build a typed hold request
6. acquire the hold and print the resource key plus token
7. forfeit the hold using `ForfeitWith(handle.Token())`
8. print a short success line and shutdown confirmation

## Documentation Changes

- add the dedicated hold example to the README learning path
- add the example to any short example lists in production-oriented docs where sync and async examples are already called out

## Verification

- compile tagged examples with `go test -tags lockman_examples ./examples/... -run '^$'`
- run focused root tests if needed for touched code paths
