# Lock Scenarios And Best Practices

## Start With One Resource

Most teams should start with:

- `BindResourceID(...)`
- one typed input struct
- one centrally registered use case
- one `Client.Run(...)` or `Client.Claim(...)` callsite

This keeps the API discoverable and avoids raw map-based key handling.

## Set Identity Once

Prefer one client identity:

```go
lockman.WithIdentity(lockman.Identity{OwnerID: "orders-api"})
```

Use per-call `lockman.OwnerID(...)` only for unusual flows that truly need an override.

## Use `Run` For Direct Calls

Good `Run` cases:

- request/response approval flows
- synchronous inventory reservation
- strict fenced writes
- composite transfers that finish inside one call

## Use `Claim` For Deliveries

Good `Claim` cases:

- queue consumers
- retryable background jobs
- event handlers with duplicate delivery risk

If you use `Claim`, wire an idempotency store from the start.

## Prefer Central Registration

Register every use case once at startup:

```go
reg := lockman.NewRegistry()
if err := reg.Register(orderlocks.Approve, orderlocks.Process); err != nil {
	return err
}
```

This is where duplicate names, missing capabilities, and backend mismatches should fail.

## Keep Keys Domain-Shaped

Resource keys should match domain concepts:

- `order:123`
- `account:acct-123`
- `ledger:ledger-456`

Avoid opaque hashes unless they are the real business identifier.

## Leave Advanced Cases Explicit

Do not start in advanced packages unless the problem really needs them.

- strict mode: fencing token semantics matter
- composite mode: one operation needs multiple coordinated resources
- lineage and guard paths: specialized platform-level cases

Historical engine-level examples can still be useful for deep dives, but new integrations should start from the root `lockman` docs and examples first.
