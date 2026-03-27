package testkit

import (
	"context"
	"errors"
	"testing"
	"time"

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
	lockerrors "lockman/lockkit/errors"
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

func TestMemoryDriverAcquireWithLineageRejectsParentWhileChildHeld(t *testing.T) {
	driver := NewMemoryDriver()

	childReq := drivers.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Second,
		Lineage: drivers.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    definitions.KindChild,
			AncestorKeys: []drivers.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}
	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}
	defer func() {
		_ = driver.ReleaseWithLineage(context.Background(), childLease, childReq.Lineage)
	}()

	_, err = driver.AcquireWithLineage(context.Background(), drivers.LineageAcquireRequest{
		DefinitionID: "order",
		ResourceKey:  "order:123",
		OwnerID:      "worker-b",
		LeaseTTL:     30 * time.Second,
		Lineage: drivers.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    definitions.KindParent,
		},
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection, got %v", err)
	}
}

func TestCheckPresenceRemainsExactKeyOnlyWithActiveChild(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := drivers.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Second,
		Lineage: drivers.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    definitions.KindChild,
			AncestorKeys: []drivers.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}
	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}
	defer func() {
		_ = driver.ReleaseWithLineage(context.Background(), childLease, childReq.Lineage)
	}()

	record, err := driver.CheckPresence(context.Background(), drivers.PresenceRequest{
		DefinitionID: "order",
		ResourceKeys: []string{"order:123"},
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}
	if record.Present {
		t.Fatalf("expected exact-key presence only, got %#v", record)
	}
}

func TestMemoryDriverRenewWithLineageExtendsDescendantMembershipTTL(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := drivers.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Millisecond,
		Lineage: drivers.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    definitions.KindChild,
			AncestorKeys: []drivers.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}

	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}
	originalExpireAt := driver.lineageLeases[childReq.Lineage.LeaseID].expireAt

	time.Sleep(10 * time.Millisecond)

	childLease.LeaseTTL = 90 * time.Millisecond
	renewedLease, renewedMeta, err := driver.RenewWithLineage(context.Background(), childLease, childReq.Lineage)
	if err != nil {
		t.Fatalf("renew failed: %v", err)
	}
	defer func() {
		_ = driver.ReleaseWithLineage(context.Background(), renewedLease, renewedMeta)
	}()

	if !driver.lineageLeases[childReq.Lineage.LeaseID].expireAt.After(originalExpireAt) {
		t.Fatalf("expected lineage lease expiry to extend beyond %v, got %v", originalExpireAt, driver.lineageLeases[childReq.Lineage.LeaseID].expireAt)
	}

	ancestorKey := formatAncestorKey(childReq.Lineage.AncestorKeys[0])
	if got := driver.descendantsByAncestor[ancestorKey][childReq.Lineage.LeaseID]; !got.Equal(renewedLease.ExpiresAt) {
		t.Fatalf("expected descendant membership expiry %v, got %v", renewedLease.ExpiresAt, got)
	}
}

func TestMemoryDriverReleaseWithLineageClearsDescendantMembership(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := drivers.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Second,
		Lineage: drivers.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    definitions.KindChild,
			AncestorKeys: []drivers.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}

	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}
	if err := driver.ReleaseWithLineage(context.Background(), childLease, childReq.Lineage); err != nil {
		t.Fatalf("release failed: %v", err)
	}

	ancestorKey := formatAncestorKey(childReq.Lineage.AncestorKeys[0])
	if got := len(driver.descendantsByAncestor[ancestorKey]); got != 0 {
		t.Fatalf("expected descendant membership cleanup, got %d entries", got)
	}

	parentLease, err := driver.AcquireWithLineage(context.Background(), drivers.LineageAcquireRequest{
		DefinitionID: "order",
		ResourceKey:  "order:123",
		OwnerID:      "worker-b",
		LeaseTTL:     30 * time.Second,
		Lineage: drivers.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    definitions.KindParent,
		},
	})
	if err != nil {
		t.Fatalf("expected parent acquire after child release, got %v", err)
	}
	if err := driver.ReleaseWithLineage(context.Background(), parentLease, drivers.LineageLeaseMeta{
		LeaseID: "lease-parent",
		Kind:    definitions.KindParent,
	}); err != nil {
		t.Fatalf("parent release failed: %v", err)
	}
}

func TestMemoryDriverAcquireWithLineagePrunesExpiredDescendantMembership(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := drivers.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     20 * time.Millisecond,
		Lineage: drivers.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    definitions.KindChild,
			AncestorKeys: []drivers.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}

	if _, err := driver.AcquireWithLineage(context.Background(), childReq); err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}

	time.Sleep(30 * time.Millisecond)

	parentLease, err := driver.AcquireWithLineage(context.Background(), drivers.LineageAcquireRequest{
		DefinitionID: "order",
		ResourceKey:  "order:123",
		OwnerID:      "worker-b",
		LeaseTTL:     30 * time.Second,
		Lineage: drivers.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    definitions.KindParent,
		},
	})
	if err != nil {
		t.Fatalf("expected parent acquire after child expiry, got %v", err)
	}
	defer func() {
		_ = driver.ReleaseWithLineage(context.Background(), parentLease, drivers.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    definitions.KindParent,
		})
	}()

	ancestorKey := formatAncestorKey(childReq.Lineage.AncestorKeys[0])
	if got := len(driver.descendantsByAncestor[ancestorKey]); got != 0 {
		t.Fatalf("expected expired descendant membership cleanup, got %d entries", got)
	}
}

func TestMemoryDriverReleaseWithLineagePreservesOtherDescendants(t *testing.T) {
	driver := NewMemoryDriver()
	firstChildReq := drivers.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Second,
		Lineage: drivers.LineageLeaseMeta{
			LeaseID: "lease-child-1",
			Kind:    definitions.KindChild,
			AncestorKeys: []drivers.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}
	secondChildReq := drivers.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-2",
		OwnerID:      "worker-b",
		LeaseTTL:     30 * time.Second,
		Lineage: drivers.LineageLeaseMeta{
			LeaseID: "lease-child-2",
			Kind:    definitions.KindChild,
			AncestorKeys: []drivers.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}

	firstLease, err := driver.AcquireWithLineage(context.Background(), firstChildReq)
	if err != nil {
		t.Fatalf("first child acquire failed: %v", err)
	}
	secondLease, err := driver.AcquireWithLineage(context.Background(), secondChildReq)
	if err != nil {
		t.Fatalf("second child acquire failed: %v", err)
	}

	if err := driver.ReleaseWithLineage(context.Background(), firstLease, firstChildReq.Lineage); err != nil {
		t.Fatalf("first child release failed: %v", err)
	}

	_, err = driver.AcquireWithLineage(context.Background(), drivers.LineageAcquireRequest{
		DefinitionID: "order",
		ResourceKey:  "order:123",
		OwnerID:      "worker-c",
		LeaseTTL:     30 * time.Second,
		Lineage: drivers.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    definitions.KindParent,
		},
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected remaining child to keep parent blocked, got %v", err)
	}

	if err := driver.ReleaseWithLineage(context.Background(), secondLease, secondChildReq.Lineage); err != nil {
		t.Fatalf("second child release failed: %v", err)
	}
}
