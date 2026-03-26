package testkit

import (
	"context"
	"testing"
	"time"

	"lockman/lockkit/drivers"
)

func TestMemoryDriverAcquireAndRelease(t *testing.T) {
	driver := NewMemoryDriver()

	lease, err := driver.Acquire(context.Background(), drivers.AcquireRequest{
		DefinitionID: "OrderLock",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "svc-a:instance-1",
		LeaseTTL:     30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	AssertSingleResourceLease(t, lease, "OrderLock", "svc-a:instance-1", "order:123")

	if err := driver.Release(context.Background(), lease); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}

	presence, err := driver.CheckPresence(context.Background(), drivers.PresenceRequest{
		DefinitionID: "OrderLock",
		ResourceKeys: []string{"order:123"},
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}

	if presence.Present {
		t.Fatalf("expected resource to be absent after release, got %+v", presence)
	}
}
