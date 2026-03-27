package drivers

import (
	"reflect"
	"testing"
	"time"
)

func TestPresenceRecordCarriesLeaseMetadata(t *testing.T) {
	now := time.Now()
	record := PresenceRecord{
		Present:      true,
		DefinitionID: "order.lock",
		ResourceKeys: []string{"order:123"},
		Lease: LeaseRecord{
			DefinitionID: "order.lock",
			ResourceKeys: []string{"order:123"},
			OwnerID:      "worker-a",
			ExpiresAt:    now.Add(time.Minute),
		},
	}

	if !record.Present {
		t.Fatal("expected presence record to be present")
	}
	if record.Lease.OwnerID != "worker-a" {
		t.Fatalf("expected lease owner metadata, got %q", record.Lease.OwnerID)
	}
	if record.Lease.ExpiresAt.IsZero() {
		t.Fatal("expected lease expiry metadata to be populated")
	}
}

func TestDriverContractHasNoInspectMethod(t *testing.T) {
	driverType := reflect.TypeOf((*Driver)(nil)).Elem()
	if _, exists := driverType.MethodByName("Inspect"); exists {
		t.Fatalf("driver contract must not expose redis-specific inspect method")
	}
}

func TestLineageDriverContractIsOptional(t *testing.T) {
	driverType := reflect.TypeOf((*Driver)(nil)).Elem()
	if _, exists := driverType.MethodByName("AcquireWithLineage"); exists {
		t.Fatalf("drivers.Driver must remain exact-key only; lineage is an optional capability")
	}

	lineageType := reflect.TypeOf((*LineageDriver)(nil)).Elem()
	for _, name := range []string{"AcquireWithLineage", "RenewWithLineage", "ReleaseWithLineage"} {
		if _, exists := lineageType.MethodByName(name); !exists {
			t.Fatalf("LineageDriver contract missing method %s", name)
		}
	}
}
