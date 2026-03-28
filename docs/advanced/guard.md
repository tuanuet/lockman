# Advanced: Guard

Guard flows are for lower-level guarded-write integrations where the storage boundary itself must enforce lock identity or fencing semantics.

Most application teams should start with:

- `client.Run(...)`
- `client.Claim(...)`
- `lockman/advanced/strict` only if they need fencing tokens

Reach for guard-oriented integrations only when you are designing a persistence boundary that must classify stale or mismatched writers explicitly.
