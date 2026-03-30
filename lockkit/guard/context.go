package guard

import (
	contractguard "lockman/guard"
	"lockman/internal/guardbridge"
	"lockman/lockkit/definitions"
)

// Context carries the strict lock identity and fencing data needed for a
// guarded write against a single persisted resource boundary.
type Context = contractguard.Context

// Outcome classifies the result of a guarded write attempt.
type Outcome = contractguard.Outcome

const (
	OutcomeApplied           Outcome = contractguard.OutcomeApplied
	OutcomeDuplicateIgnored  Outcome = contractguard.OutcomeDuplicateIgnored
	OutcomeStaleRejected     Outcome = contractguard.OutcomeStaleRejected
	OutcomeVersionConflict   Outcome = contractguard.OutcomeVersionConflict
	OutcomeInvariantRejected Outcome = contractguard.OutcomeInvariantRejected
)

// ContextFromLease maps a single-resource lease into a guarded-write context.
// Composite guarded-write behavior remains out of scope for Phase 3b.
func ContextFromLease(lease definitions.LeaseContext) Context {
	return guardbridge.FromLeaseContext(lease)
}

// ContextFromClaim maps a single-resource worker claim into a guarded-write
// context. Composite guarded-write behavior remains out of scope for Phase 3b.
func ContextFromClaim(claim definitions.ClaimContext) Context {
	return guardbridge.FromClaimContext(claim)
}
