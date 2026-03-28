package guard_test

import (
	"testing"

	"lockman/lockkit/definitions"
	"lockman/lockkit/guard"
)

func TestContextFromClaimExactFieldMapping(t *testing.T) {
	claim := definitions.ClaimContext{
		DefinitionID: "StrictOrderClaim",
		ResourceKey:  "order:123",
		Ownership: definitions.OwnershipMeta{
			OwnerID:   "worker-a",
			MessageID: "msg-123",
		},
		FencingToken:   7,
		IdempotencyKey: "idem-123",
	}

	got := guard.ContextFromClaim(claim)
	want := guard.Context{
		LockID:         "StrictOrderClaim",
		ResourceKey:    "order:123",
		FencingToken:   7,
		OwnerID:        "worker-a",
		MessageID:      "msg-123",
		IdempotencyKey: "idem-123",
	}

	if got != want {
		t.Fatalf("unexpected guard context: %#v", got)
	}
}

func TestContextFromLeaseExactFieldMappingAndWorkerFieldsZeroValue(t *testing.T) {
	lease := definitions.LeaseContext{
		DefinitionID: "StrictOrderLock",
		ResourceKey:  "order:123",
		Ownership: definitions.OwnershipMeta{
			OwnerID: "runtime-a",
		},
		FencingToken: 11,
	}

	got := guard.ContextFromLease(lease)
	want := guard.Context{
		LockID:       "StrictOrderLock",
		ResourceKey:  "order:123",
		FencingToken: 11,
		OwnerID:      "runtime-a",
	}

	if got != want {
		t.Fatalf("unexpected guard context: %#v", got)
	}

	if got.MessageID != "" || got.IdempotencyKey != "" {
		t.Fatalf("expected zero-value runtime-only fields, got %#v", got)
	}
}

func TestOutcomeStringsRemainStable(t *testing.T) {
	cases := map[guard.Outcome]string{
		guard.OutcomeApplied:           "applied",
		guard.OutcomeDuplicateIgnored:  "duplicate_ignored",
		guard.OutcomeStaleRejected:     "stale_rejected",
		guard.OutcomeVersionConflict:   "version_conflict",
		guard.OutcomeInvariantRejected: "invariant_rejected",
	}

	for outcome, want := range cases {
		if string(outcome) != want {
			t.Fatalf("outcome %q changed, want %q", outcome, want)
		}
	}
}
