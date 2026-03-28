package guard

import "lockman/lockkit/definitions"

// Context carries the strict lock identity and fencing data needed for a
// guarded write against a single persisted resource boundary.
type Context struct {
	LockID         string
	ResourceKey    string
	FencingToken   uint64
	OwnerID        string
	MessageID      string
	IdempotencyKey string
}

// Outcome classifies the result of a guarded write attempt.
type Outcome string

const (
	OutcomeApplied           Outcome = "applied"
	OutcomeDuplicateIgnored  Outcome = "duplicate_ignored"
	OutcomeStaleRejected     Outcome = "stale_rejected"
	OutcomeVersionConflict   Outcome = "version_conflict"
	OutcomeInvariantRejected Outcome = "invariant_rejected"
)

// ContextFromLease maps a single-resource lease into a guarded-write context.
// Composite guarded-write behavior remains out of scope for Phase 3b.
func ContextFromLease(lease definitions.LeaseContext) Context {
	return Context{
		LockID:       lease.DefinitionID,
		ResourceKey:  lease.ResourceKey,
		FencingToken: lease.FencingToken,
		OwnerID:      lease.Ownership.OwnerID,
	}
}

// ContextFromClaim maps a single-resource worker claim into a guarded-write
// context. Composite guarded-write behavior remains out of scope for Phase 3b.
func ContextFromClaim(claim definitions.ClaimContext) Context {
	return Context{
		LockID:         claim.DefinitionID,
		ResourceKey:    claim.ResourceKey,
		FencingToken:   claim.FencingToken,
		OwnerID:        claim.Ownership.OwnerID,
		MessageID:      claim.Ownership.MessageID,
		IdempotencyKey: claim.IdempotencyKey,
	}
}
