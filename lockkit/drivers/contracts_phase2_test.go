package drivers

import (
	"context"
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
	for _, name := range []string{"AcquireWithLineage", "RenewWithLineage", "ReleaseWithLineage"} {
		if _, exists := driverType.MethodByName(name); exists {
			t.Fatalf("drivers.Driver must remain exact-key only; lineage is an optional capability")
		}
	}

	lineageType := reflect.TypeOf((*LineageDriver)(nil)).Elem()
	acquireMethod, ok := lineageType.MethodByName("AcquireWithLineage")
	if !ok {
		t.Fatal("LineageDriver contract missing method AcquireWithLineage")
	}
	if got, want := acquireMethod.Type.NumIn(), 2; got != want {
		t.Fatalf("AcquireWithLineage expected %d inputs, got %d", want, got)
	}
	if got, want := acquireMethod.Type.NumOut(), 2; got != want {
		t.Fatalf("AcquireWithLineage expected %d outputs, got %d", want, got)
	}
	if got, want := acquireMethod.Type.In(0), reflect.TypeOf((*context.Context)(nil)).Elem(); got != want {
		t.Fatalf("AcquireWithLineage arg0 type = %v, want %v", got, want)
	}
	if got, want := acquireMethod.Type.In(1), reflect.TypeOf(LineageAcquireRequest{}); got != want {
		t.Fatalf("AcquireWithLineage arg1 type = %v, want %v", got, want)
	}
	if got, want := acquireMethod.Type.Out(0), reflect.TypeOf(LeaseRecord{}); got != want {
		t.Fatalf("AcquireWithLineage out0 type = %v, want %v", got, want)
	}
	if got, want := acquireMethod.Type.Out(1), reflect.TypeOf((*error)(nil)).Elem(); got != want {
		t.Fatalf("AcquireWithLineage out1 type = %v, want %v", got, want)
	}

	renewMethod, ok := lineageType.MethodByName("RenewWithLineage")
	if !ok {
		t.Fatal("LineageDriver contract missing method RenewWithLineage")
	}
	if got, want := renewMethod.Type.NumIn(), 3; got != want {
		t.Fatalf("RenewWithLineage expected %d inputs, got %d", want, got)
	}
	if got, want := renewMethod.Type.NumOut(), 3; got != want {
		t.Fatalf("RenewWithLineage expected %d outputs, got %d", want, got)
	}
	if got, want := renewMethod.Type.In(0), reflect.TypeOf((*context.Context)(nil)).Elem(); got != want {
		t.Fatalf("RenewWithLineage arg0 type = %v, want %v", got, want)
	}
	if got, want := renewMethod.Type.In(1), reflect.TypeOf(LeaseRecord{}); got != want {
		t.Fatalf("RenewWithLineage arg1 type = %v, want %v", got, want)
	}
	if got, want := renewMethod.Type.In(2), reflect.TypeOf(LineageLeaseMeta{}); got != want {
		t.Fatalf("RenewWithLineage arg2 type = %v, want %v", got, want)
	}
	if got, want := renewMethod.Type.Out(0), reflect.TypeOf(LeaseRecord{}); got != want {
		t.Fatalf("RenewWithLineage out0 type = %v, want %v", got, want)
	}
	if got, want := renewMethod.Type.Out(1), reflect.TypeOf(LineageLeaseMeta{}); got != want {
		t.Fatalf("RenewWithLineage out1 type = %v, want %v", got, want)
	}
	if got, want := renewMethod.Type.Out(2), reflect.TypeOf((*error)(nil)).Elem(); got != want {
		t.Fatalf("RenewWithLineage out2 type = %v, want %v", got, want)
	}

	releaseMethod, ok := lineageType.MethodByName("ReleaseWithLineage")
	if !ok {
		t.Fatal("LineageDriver contract missing method ReleaseWithLineage")
	}
	if got, want := releaseMethod.Type.NumIn(), 3; got != want {
		t.Fatalf("ReleaseWithLineage expected %d inputs, got %d", want, got)
	}
	if got, want := releaseMethod.Type.NumOut(), 1; got != want {
		t.Fatalf("ReleaseWithLineage expected %d outputs, got %d", want, got)
	}
	if got, want := releaseMethod.Type.In(0), reflect.TypeOf((*context.Context)(nil)).Elem(); got != want {
		t.Fatalf("ReleaseWithLineage arg0 type = %v, want %v", got, want)
	}
	if got, want := releaseMethod.Type.In(1), reflect.TypeOf(LeaseRecord{}); got != want {
		t.Fatalf("ReleaseWithLineage arg1 type = %v, want %v", got, want)
	}
	if got, want := releaseMethod.Type.In(2), reflect.TypeOf(LineageLeaseMeta{}); got != want {
		t.Fatalf("ReleaseWithLineage arg2 type = %v, want %v", got, want)
	}
	if got, want := releaseMethod.Type.Out(0), reflect.TypeOf((*error)(nil)).Elem(); got != want {
		t.Fatalf("ReleaseWithLineage out0 type = %v, want %v", got, want)
	}
}
