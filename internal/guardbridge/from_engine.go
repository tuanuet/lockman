package guardbridge

import (
	"lockman/guard"
	"lockman/lockkit/definitions"
)

// FromLeaseContext maps a single-resource lease into a guarded-write context.
// Composite guarded-write behavior remains out of scope for Phase 3b.
func FromLeaseContext(lease definitions.LeaseContext) guard.Context {
	return guard.Context{
		LockID:       lease.DefinitionID,
		ResourceKey:  lease.ResourceKey,
		FencingToken: lease.FencingToken,
		OwnerID:      lease.Ownership.OwnerID,
	}
}

// FromClaimContext maps a single-resource worker claim into a guarded-write
// context. Composite guarded-write behavior remains out of scope for Phase 3b.
func FromClaimContext(claim definitions.ClaimContext) guard.Context {
	return guard.Context{
		LockID:         claim.DefinitionID,
		ResourceKey:    claim.ResourceKey,
		FencingToken:   claim.FencingToken,
		OwnerID:        claim.Ownership.OwnerID,
		MessageID:      claim.Ownership.MessageID,
		IdempotencyKey: claim.IdempotencyKey,
	}
}
