# Advanced: Guard

Guard flows are for lower-level guarded-write integrations where the storage boundary itself must enforce lock identity or fencing semantics.

The stable contract types for guarded-write adapters live in the top-level `github.com/tuanuet/lockman/guard` package. The `github.com/tuanuet/lockman/advanced/guard` namespace is an advanced path for higher-level composition and policy beyond the default client flows.

Most application teams should start with:

- `client.Run(...)`
- `client.Claim(...)`
- `github.com/tuanuet/lockman/advanced/strict` only if they need fencing tokens

Reach for guard-oriented integrations only when you are designing a persistence boundary that must classify stale or mismatched writers explicitly.
