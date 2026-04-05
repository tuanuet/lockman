package memory

import (
	"testing"

	"github.com/tuanuet/lockman/backend"
)

// AssertSingleResourceLease ensures a lease record matches a single key expectation.
func AssertSingleResourceLease(t *testing.T, lease backend.LeaseRecord, defID, ownerID, resourceKey string) {
	t.Helper()

	if lease.DefinitionID != defID {
		t.Fatalf("expected definition %q, got %q", defID, lease.DefinitionID)
	}

	if lease.OwnerID != ownerID {
		t.Fatalf("expected owner %q, got %q", ownerID, lease.OwnerID)
	}

	if len(lease.ResourceKeys) != 1 {
		t.Fatalf("expected 1 resource key, got %d", len(lease.ResourceKeys))
	}

	if lease.ResourceKeys[0] != resourceKey {
		t.Fatalf("expected resource key %q, got %q", resourceKey, lease.ResourceKeys[0])
	}
}
