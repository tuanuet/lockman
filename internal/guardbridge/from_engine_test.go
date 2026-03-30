package guardbridge_test

import (
	"testing"

	"github.com/tuanuet/lockman/guard"
	"github.com/tuanuet/lockman/internal/guardbridge"
	"github.com/tuanuet/lockman/lockkit/definitions"
)

func TestFromClaimContextExactFieldMapping(t *testing.T) {
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

	got := guardbridge.FromClaimContext(claim)
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

func TestFromLeaseContextExactFieldMappingAndWorkerFieldsZeroValue(t *testing.T) {
	lease := definitions.LeaseContext{
		DefinitionID: "StrictOrderLock",
		ResourceKey:  "order:123",
		Ownership: definitions.OwnershipMeta{
			OwnerID: "runtime-a",
		},
		FencingToken: 11,
	}

	got := guardbridge.FromLeaseContext(lease)
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
