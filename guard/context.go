package guard

import "errors"

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

var (
	// ErrInvariantRejected indicates a guarded write was rejected because the
	// persisted boundary did not satisfy the strict-lock invariants.
	ErrInvariantRejected = errors.New("invariant rejected")
)
