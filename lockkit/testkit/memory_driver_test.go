package testkit

import (
	"context"
	"errors"
	"testing"
	"time"

	"lockman/backend"
	lockerrors "lockman/lockkit/errors"
)

func requireStrictDriver(t *testing.T, driver *MemoryDriver) backend.StrictDriver {
	t.Helper()

	strict, ok := any(driver).(backend.StrictDriver)
	if !ok {
		t.Fatal("memory driver must implement backend.StrictDriver")
	}
	return strict
}

func TestMemoryDriverAcquireStrictIssuesIncreasingTokens(t *testing.T) {
	driver := NewMemoryDriver()
	strict := requireStrictDriver(t, driver)
	ctx := context.Background()

	first, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict first returned error: %v", err)
	}
	if err := strict.ReleaseStrict(ctx, first.Lease, first.FencingToken); err != nil {
		t.Fatalf("ReleaseStrict first returned error: %v", err)
	}

	second, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict",
		ResourceKey:  "order:123",
		OwnerID:      "worker-b",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict second returned error: %v", err)
	}
	if second.FencingToken <= first.FencingToken {
		t.Fatalf("expected monotonic fencing tokens, first=%d second=%d", first.FencingToken, second.FencingToken)
	}
}

func TestMemoryDriverRenewStrictPreservesToken(t *testing.T) {
	driver := NewMemoryDriver()
	strict := requireStrictDriver(t, driver)
	ctx := context.Background()

	acquired, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("AcquireStrict returned error: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	acquired.Lease.LeaseTTL = 40 * time.Millisecond
	renewed, err := strict.RenewStrict(ctx, acquired.Lease, acquired.FencingToken)
	if err != nil {
		t.Fatalf("RenewStrict returned error: %v", err)
	}
	if renewed.FencingToken != acquired.FencingToken {
		t.Fatalf("expected RenewStrict to preserve token %d, got %d", acquired.FencingToken, renewed.FencingToken)
	}
	if !renewed.Lease.ExpiresAt.After(acquired.Lease.ExpiresAt) {
		t.Fatalf("expected renewed expiry after %v, got %v", acquired.Lease.ExpiresAt, renewed.Lease.ExpiresAt)
	}
}

func TestMemoryDriverRenewStrictRejectsWrongToken(t *testing.T) {
	driver := NewMemoryDriver()
	strict := requireStrictDriver(t, driver)
	ctx := context.Background()

	acquired, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict returned error: %v", err)
	}

	_, err = strict.RenewStrict(ctx, acquired.Lease, acquired.FencingToken+1)
	if !errors.Is(err, backend.ErrLeaseOwnerMismatch) {
		t.Fatalf("expected ErrLeaseOwnerMismatch for wrong renew token, got %v", err)
	}
}

func TestMemoryDriverReleaseStrictRejectsWrongToken(t *testing.T) {
	driver := NewMemoryDriver()
	strict := requireStrictDriver(t, driver)
	ctx := context.Background()

	acquired, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict returned error: %v", err)
	}

	err = strict.ReleaseStrict(ctx, acquired.Lease, acquired.FencingToken+1)
	if !errors.Is(err, backend.ErrLeaseOwnerMismatch) {
		t.Fatalf("expected ErrLeaseOwnerMismatch for wrong token, got %v", err)
	}
}

func TestMemoryDriverAcquireStrictCounterIsScopedByDefinitionAndResource(t *testing.T) {
	driver := NewMemoryDriver()
	strict := requireStrictDriver(t, driver)
	ctx := context.Background()

	defA1, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict.a",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict defA returned error: %v", err)
	}
	if err := strict.ReleaseStrict(ctx, defA1.Lease, defA1.FencingToken); err != nil {
		t.Fatalf("ReleaseStrict defA returned error: %v", err)
	}

	defB1, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict.b",
		ResourceKey:  "order:123",
		OwnerID:      "worker-b",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict defB returned error: %v", err)
	}
	if defB1.FencingToken != 1 {
		t.Fatalf("expected independent counter for definition/resource boundary, got %d", defB1.FencingToken)
	}
}

func TestMemoryDriverReleaseStrictRejectsStaleLeaseFromDifferentDefinitionBoundary(t *testing.T) {
	driver := NewMemoryDriver()
	strict := requireStrictDriver(t, driver)
	ctx := context.Background()

	first, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict.a",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict first returned error: %v", err)
	}
	if err := strict.ReleaseStrict(ctx, first.Lease, first.FencingToken); err != nil {
		t.Fatalf("ReleaseStrict first returned error: %v", err)
	}

	second, err := strict.AcquireStrict(ctx, backend.StrictAcquireRequest{
		DefinitionID: "order.strict.b",
		ResourceKey:  "order:123",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireStrict second returned error: %v", err)
	}

	err = strict.ReleaseStrict(ctx, first.Lease, first.FencingToken)
	if !errors.Is(err, backend.ErrLeaseOwnerMismatch) {
		t.Fatalf("expected stale cross-definition release to fail, got %v", err)
	}

	if err := strict.ReleaseStrict(ctx, second.Lease, second.FencingToken); err != nil {
		t.Fatalf("ReleaseStrict second returned error: %v", err)
	}
}

func TestMemoryDriverAcquireStrictRejectsEmptyResourceKey(t *testing.T) {
	driver := NewMemoryDriver()
	strict := requireStrictDriver(t, driver)

	_, err := strict.AcquireStrict(context.Background(), backend.StrictAcquireRequest{
		DefinitionID: "order.strict",
		ResourceKey:  "",
		OwnerID:      "worker-a",
		LeaseTTL:     time.Second,
	})
	if !errors.Is(err, backend.ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest for empty resource key, got %v", err)
	}
}

func TestMemoryDriverAcquireAndRelease(t *testing.T) {
	driver := NewMemoryDriver()

	lease, err := driver.Acquire(context.Background(), backend.AcquireRequest{
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

	presence, err := driver.CheckPresence(context.Background(), backend.PresenceRequest{
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

	childReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
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

	_, err = driver.AcquireWithLineage(context.Background(), backend.LineageAcquireRequest{
		DefinitionID: "order",
		ResourceKey:  "order:123",
		OwnerID:      "worker-b",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    backend.KindParent,
		},
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection, got %v", err)
	}
}

func TestCheckPresenceRemainsExactKeyOnlyWithActiveChild(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
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

	record, err := driver.CheckPresence(context.Background(), backend.PresenceRequest{
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
	childReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Millisecond,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
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

func TestMemoryDriverRenewWithLineageFailsWhenLineageStateMissing(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     25 * time.Millisecond,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}

	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}

	delete(driver.lineageLeases, childReq.Lineage.LeaseID)

	childLease.LeaseTTL = 80 * time.Millisecond
	_, _, err = driver.RenewWithLineage(context.Background(), childLease, childReq.Lineage)
	if !errors.Is(err, backend.ErrLeaseExpired) {
		t.Fatalf("expected renew failure when lineage state is missing, got %v", err)
	}

	time.Sleep(35 * time.Millisecond)

	reacquired, err := driver.AcquireWithLineage(context.Background(), backend.LineageAcquireRequest{
		DefinitionID: childReq.DefinitionID,
		ResourceKey:  childReq.ResourceKey,
		OwnerID:      "worker-b",
		LeaseTTL:     50 * time.Millisecond,
		Lineage: backend.LineageLeaseMeta{
			LeaseID:      "lease-child-reacquired",
			Kind:         childReq.Lineage.Kind,
			AncestorKeys: append([]backend.AncestorKey(nil), childReq.Lineage.AncestorKeys...),
		},
	})
	if err != nil {
		t.Fatalf("expected child lease to expire on original ttl, got %v", err)
	}
	if err := driver.ReleaseWithLineage(context.Background(), reacquired, backend.LineageLeaseMeta{
		LeaseID:      "lease-child-reacquired",
		Kind:         childReq.Lineage.Kind,
		AncestorKeys: append([]backend.AncestorKey(nil), childReq.Lineage.AncestorKeys...),
	}); err != nil {
		t.Fatalf("release reacquired child failed: %v", err)
	}
}

func TestMemoryDriverRenewWithLineageFailsWhenAncestorMembershipMissing(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     25 * time.Millisecond,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}

	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}

	ancestorKey := formatAncestorKey(childReq.Lineage.AncestorKeys[0])
	delete(driver.descendantsByAncestor[ancestorKey], childReq.Lineage.LeaseID)

	childLease.LeaseTTL = 80 * time.Millisecond
	_, _, err = driver.RenewWithLineage(context.Background(), childLease, childReq.Lineage)
	if !errors.Is(err, backend.ErrLeaseExpired) {
		t.Fatalf("expected renew failure when ancestor membership is missing, got %v", err)
	}

	time.Sleep(35 * time.Millisecond)

	reacquired, err := driver.AcquireWithLineage(context.Background(), backend.LineageAcquireRequest{
		DefinitionID: childReq.DefinitionID,
		ResourceKey:  childReq.ResourceKey,
		OwnerID:      "worker-b",
		LeaseTTL:     50 * time.Millisecond,
		Lineage: backend.LineageLeaseMeta{
			LeaseID:      "lease-child-reacquired",
			Kind:         childReq.Lineage.Kind,
			AncestorKeys: append([]backend.AncestorKey(nil), childReq.Lineage.AncestorKeys...),
		},
	})
	if err != nil {
		t.Fatalf("expected child lease to expire on original ttl, got %v", err)
	}
	if err := driver.ReleaseWithLineage(context.Background(), reacquired, backend.LineageLeaseMeta{
		LeaseID:      "lease-child-reacquired",
		Kind:         childReq.Lineage.Kind,
		AncestorKeys: append([]backend.AncestorKey(nil), childReq.Lineage.AncestorKeys...),
	}); err != nil {
		t.Fatalf("release reacquired child failed: %v", err)
	}
}

func TestMemoryDriverReleaseWithLineageClearsDescendantMembership(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
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

	parentLease, err := driver.AcquireWithLineage(context.Background(), backend.LineageAcquireRequest{
		DefinitionID: "order",
		ResourceKey:  "order:123",
		OwnerID:      "worker-b",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    backend.KindParent,
		},
	})
	if err != nil {
		t.Fatalf("expected parent acquire after child release, got %v", err)
	}
	if err := driver.ReleaseWithLineage(context.Background(), parentLease, backend.LineageLeaseMeta{
		LeaseID: "lease-parent",
		Kind:    backend.KindParent,
	}); err != nil {
		t.Fatalf("parent release failed: %v", err)
	}
}

func TestMemoryDriverAcquireWithLineagePrunesExpiredDescendantMembership(t *testing.T) {
	driver := NewMemoryDriver()
	childReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     20 * time.Millisecond,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}

	if _, err := driver.AcquireWithLineage(context.Background(), childReq); err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}

	time.Sleep(30 * time.Millisecond)

	parentLease, err := driver.AcquireWithLineage(context.Background(), backend.LineageAcquireRequest{
		DefinitionID: "order",
		ResourceKey:  "order:123",
		OwnerID:      "worker-b",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    backend.KindParent,
		},
	})
	if err != nil {
		t.Fatalf("expected parent acquire after child expiry, got %v", err)
	}
	defer func() {
		_ = driver.ReleaseWithLineage(context.Background(), parentLease, backend.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    backend.KindParent,
		})
	}()

	ancestorKey := formatAncestorKey(childReq.Lineage.AncestorKeys[0])
	if got := len(driver.descendantsByAncestor[ancestorKey]); got != 0 {
		t.Fatalf("expected expired descendant membership cleanup, got %d entries", got)
	}
}

func TestMemoryDriverReleaseWithLineagePreservesOtherDescendants(t *testing.T) {
	driver := NewMemoryDriver()
	firstChildReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "worker-a",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child-1",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "order", ResourceKey: "order:123"},
			},
		},
	}
	secondChildReq := backend.LineageAcquireRequest{
		DefinitionID: "item",
		ResourceKey:  "order:123:item:line-2",
		OwnerID:      "worker-b",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child-2",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
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

	_, err = driver.AcquireWithLineage(context.Background(), backend.LineageAcquireRequest{
		DefinitionID: "order",
		ResourceKey:  "order:123",
		OwnerID:      "worker-c",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-parent",
			Kind:    backend.KindParent,
		},
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected remaining child to keep parent blocked, got %v", err)
	}

	if err := driver.ReleaseWithLineage(context.Background(), secondLease, secondChildReq.Lineage); err != nil {
		t.Fatalf("second child release failed: %v", err)
	}
}
