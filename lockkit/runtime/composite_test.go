package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/registry"
	"github.com/tuanuet/lockman/lockkit/testkit"
)

func TestExecuteCompositeExclusiveAcquiresMembersInCanonicalOrder(t *testing.T) {
	reg := newCompositeRegistry(t)
	driver := testkit.NewMemoryDriver()
	mgr, err := NewManager(reg, driver, nil)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	var visited []string
	err = mgr.ExecuteCompositeExclusive(context.Background(), compositeRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
		visited = append(visited, lease.ResourceKeys...)
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteCompositeExclusive returned error: %v", err)
	}
	if len(visited) != 2 {
		t.Fatalf("expected 2 composite members, got %d", len(visited))
	}
	if visited[0] != "account:acct-123" || visited[1] != "ledger:ledger-456" {
		t.Fatalf("expected canonical order [account:acct-123 ledger:ledger-456], got %v", visited)
	}
}

func TestExecuteCompositeExclusiveCanonicalOrderingUsesRankThenResourceThenKey(t *testing.T) {
	reg := newCanonicalOrderingCoverageRegistry(t)
	driver := testkit.NewMemoryDriver()
	mgr, err := NewManager(reg, driver, nil)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	var visited []string
	err = mgr.ExecuteCompositeExclusive(context.Background(), definitions.CompositeLockRequest{
		DefinitionID: "OrderingCoverageComposite",
		MemberInputs: []map[string]string{
			{"id": "b"},
			{"id": "c"},
			{"id": "a"},
			{"id": "9"},
			{"id": "a"},
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		visited = append(visited, lease.ResourceKeys...)
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteCompositeExclusive returned error: %v", err)
	}

	want := []string{"zeta:9", "alpha:a", "gamma:a", "gamma:b", "gamma:c"}
	if len(visited) != len(want) {
		t.Fatalf("expected %d composite members, got %d (%v)", len(want), len(visited), visited)
	}
	for i := range want {
		if visited[i] != want[i] {
			t.Fatalf("unexpected canonical ordering, want %v got %v", want, visited)
		}
	}
}

func TestExecuteCompositeExclusiveInvalidOverridesRejectedBeforeAcquire(t *testing.T) {
	reg := newCompositeRegistry(t)
	driver := testkit.NewMemoryDriver()
	rec := &countingRecorder{}
	mgr, err := NewManager(reg, driver, rec)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteCompositeExclusive(context.Background(), definitions.CompositeLockRequest{
		DefinitionID: "TransferComposite",
		MemberInputs: []map[string]string{
			{"ledger_id": "ledger-456"},
			{"account_id": "acct-123"},
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
		Overrides: &definitions.RuntimeOverrides{
			MaxRetries: maxRetriesPtr(1),
		},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		t.Fatal("callback should not run when overrides are invalid")
		return nil
	})

	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation for invalid overrides, got %v", err)
	}
	if got := len(rec.activeCounts()); got != 0 {
		t.Fatalf("expected invalid overrides to fail before guard activity metrics, got %d events", got)
	}
}

func TestExecuteCompositeExclusiveReleasesAcquiredMembersInReverseOrderOnFailure(t *testing.T) {
	reg := newRollbackCompositeRegistry(t)
	driver := newFailingCompositeDriver(3, backend.ErrLeaseAlreadyHeld)
	mgr, err := NewManager(reg, driver, nil)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteCompositeExclusive(context.Background(), rollbackCompositeRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
		t.Fatal("callback should not run when acquire fails")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrLockBusy) {
		t.Fatalf("expected lock busy after failed member acquire, got %v", err)
	}

	released := driver.releasedKeys()
	if len(released) != 2 {
		t.Fatalf("expected two rollback releases, got %v", released)
	}
	if released[0] != "rank2:b" || released[1] != "rank1:a" {
		t.Fatalf("expected reverse release order [rank2:b rank1:a], got %v", released)
	}
}

func TestExecuteCompositeExclusiveRejectsParentChildOverlap(t *testing.T) {
	reg := newOverlapCompositeRegistry(t)
	driver := newFailingCompositeDriver(0, nil)
	mgr, err := NewManager(reg, driver, nil)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	called := false
	err = mgr.ExecuteCompositeExclusive(context.Background(), definitions.CompositeLockRequest{
		DefinitionID: "OrderComposite",
		MemberInputs: []map[string]string{
			{
				"order_id": "123",
			},
			{
				"order_id": "123",
				"item_id":  "line-1",
			},
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		called = true
		return nil
	})

	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation for overlap, got %v", err)
	}
	if called {
		t.Fatal("callback should not run when overlap is rejected")
	}
	if attempts := driver.acquireAttempts(); attempts != 0 {
		t.Fatalf("expected overlap rejection before acquire attempts, got %d", attempts)
	}
}

func TestExecuteCompositeExclusiveUsesLineageDriverForLineageMembers(t *testing.T) {
	reg := registryWithCompositeLineageMembers(t)
	driver := testkit.NewMemoryDriver()

	holder, err := NewManager(reg, driver, nil)
	if err != nil {
		t.Fatalf("holder init failed: %v", err)
	}
	compositeMgr, err := NewManager(reg, driver, nil)
	if err != nil {
		t.Fatalf("composite manager init failed: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- holder.ExecuteExclusive(context.Background(), childSyncRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	err = compositeMgr.ExecuteCompositeExclusive(context.Background(), compositeParentMemberRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
		t.Fatal("composite callback should not run while child is held")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected composite overlap rejection, got %v", err)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("child holder returned error: %v", err)
	}

	entered = make(chan struct{})
	release = make(chan struct{})
	done = make(chan error, 1)
	go func() {
		done <- holder.ExecuteExclusive(context.Background(), parentSyncRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	err = compositeMgr.ExecuteCompositeExclusive(context.Background(), compositeChildMemberRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
		t.Fatal("composite callback should not run while parent is held")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected composite overlap rejection, got %v", err)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("parent holder returned error: %v", err)
	}
}

func newCompositeRegistry(t *testing.T) *registry.Registry {
	t.Helper()

	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "LedgerMember",
		Kind:          definitions.KindParent,
		Resource:      "ledger",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          20,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("ledger:{ledger_id}", []string{"ledger_id"}),
	}); err != nil {
		t.Fatalf("register LedgerMember failed: %v", err)
	}
	if err := reg.Register(definitions.LockDefinition{
		ID:            "AccountMember",
		Kind:          definitions.KindParent,
		Resource:      "account",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          10,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("account:{account_id}", []string{"account_id"}),
	}); err != nil {
		t.Fatalf("register AccountMember failed: %v", err)
	}

	if err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:               "TransferComposite",
		Members:          []string{"LedgerMember", "AccountMember"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   2,
		ExecutionKind:    definitions.ExecutionSync,
	}); err != nil {
		t.Fatalf("register TransferComposite failed: %v", err)
	}

	return reg
}

func compositeRequest() definitions.CompositeLockRequest {
	return definitions.CompositeLockRequest{
		DefinitionID: "TransferComposite",
		MemberInputs: []map[string]string{
			{
				"ledger_id": "ledger-456",
			},
			{
				"account_id": "acct-123",
			},
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	}
}

func newRollbackCompositeRegistry(t *testing.T) *registry.Registry {
	t.Helper()

	reg := registry.New()
	register := func(def definitions.LockDefinition) {
		if err := reg.Register(def); err != nil {
			t.Fatalf("register %s failed: %v", def.ID, err)
		}
	}

	register(definitions.LockDefinition{
		ID:            "Rank3",
		Kind:          definitions.KindParent,
		Resource:      "rank3",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          3,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("rank3:{id}", []string{"id"}),
	})
	register(definitions.LockDefinition{
		ID:            "Rank2",
		Kind:          definitions.KindParent,
		Resource:      "rank2",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          2,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("rank2:{id}", []string{"id"}),
	})
	register(definitions.LockDefinition{
		ID:            "Rank1",
		Kind:          definitions.KindParent,
		Resource:      "rank1",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          1,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("rank1:{id}", []string{"id"}),
	})

	if err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:               "RollbackComposite",
		Members:          []string{"Rank3", "Rank2", "Rank1"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   3,
		ExecutionKind:    definitions.ExecutionSync,
	}); err != nil {
		t.Fatalf("register RollbackComposite failed: %v", err)
	}

	return reg
}

func newCanonicalOrderingCoverageRegistry(t *testing.T) *registry.Registry {
	t.Helper()

	reg := registry.New()
	register := func(def definitions.LockDefinition) {
		if err := reg.Register(def); err != nil {
			t.Fatalf("register %s failed: %v", def.ID, err)
		}
	}

	register(definitions.LockDefinition{
		ID:            "RankOneGammaB",
		Kind:          definitions.KindParent,
		Resource:      "gamma",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          1,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("gamma:{id}", []string{"id"}),
	})
	register(definitions.LockDefinition{
		ID:            "RankOneGammaC",
		Kind:          definitions.KindParent,
		Resource:      "gamma",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          1,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("gamma:{id}", []string{"id"}),
	})
	register(definitions.LockDefinition{
		ID:            "RankOneAlpha",
		Kind:          definitions.KindParent,
		Resource:      "alpha",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          1,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("alpha:{id}", []string{"id"}),
	})
	register(definitions.LockDefinition{
		ID:            "RankZeroZeta",
		Kind:          definitions.KindParent,
		Resource:      "zeta",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          0,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("zeta:{id}", []string{"id"}),
	})
	register(definitions.LockDefinition{
		ID:            "RankOneGammaA",
		Kind:          definitions.KindParent,
		Resource:      "gamma",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          1,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("gamma:{id}", []string{"id"}),
	})

	if err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:               "OrderingCoverageComposite",
		Members:          []string{"RankOneGammaB", "RankOneGammaC", "RankOneAlpha", "RankZeroZeta", "RankOneGammaA"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   5,
		ExecutionKind:    definitions.ExecutionSync,
	}); err != nil {
		t.Fatalf("register OrderingCoverageComposite failed: %v", err)
	}

	return reg
}

func rollbackCompositeRequest() definitions.CompositeLockRequest {
	return definitions.CompositeLockRequest{
		DefinitionID: "RollbackComposite",
		MemberInputs: []map[string]string{
			{
				"id": "c",
			},
			{
				"id": "b",
			},
			{
				"id": "a",
			},
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	}
}

func newOverlapCompositeRegistry(t *testing.T) *registry.Registry {
	t.Helper()

	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderParent",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          10,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register OrderParent failed: %v", err)
	}
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderChild",
		Kind:          definitions.KindChild,
		ParentRef:     "OrderParent",
		OverlapPolicy: definitions.OverlapReject,
		Resource:      "order_line",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		Rank:          20,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}:{item_id}", []string{"order_id", "item_id"}),
	}); err != nil {
		t.Fatalf("register OrderChild failed: %v", err)
	}

	if err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:               "OrderComposite",
		Members:          []string{"OrderParent", "OrderChild"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   2,
		ExecutionKind:    definitions.ExecutionSync,
	}); err != nil {
		t.Fatalf("register OrderComposite failed: %v", err)
	}

	return reg
}

func registryWithCompositeLineageMembers(t *testing.T) *registry.Registry {
	t.Helper()

	reg := registryWithLineageChain(t)
	register := func(def definitions.CompositeDefinition) {
		if err := reg.RegisterComposite(def); err != nil {
			t.Fatalf("register %s failed: %v", def.ID, err)
		}
	}

	register(definitions.CompositeDefinition{
		ID:               "ParentOnlyComposite",
		Members:          []string{"OrderLock"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   1,
		ExecutionKind:    definitions.ExecutionSync,
	})
	register(definitions.CompositeDefinition{
		ID:               "ChildOnlyComposite",
		Members:          []string{"ItemLock"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   1,
		ExecutionKind:    definitions.ExecutionSync,
	})

	return reg
}

func compositeParentMemberRequest() definitions.CompositeLockRequest {
	return definitions.CompositeLockRequest{
		DefinitionID: "ParentOnlyComposite",
		MemberInputs: []map[string]string{
			{
				"order_id": "123",
			},
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:composite-parent"},
	}
}

func compositeChildMemberRequest() definitions.CompositeLockRequest {
	return definitions.CompositeLockRequest{
		DefinitionID: "ChildOnlyComposite",
		MemberInputs: []map[string]string{
			{
				"order_id": "123",
				"item_id":  "line-1",
			},
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:composite-child"},
	}
}

type failingCompositeDriver struct {
	mu              sync.Mutex
	failAt          int
	failErr         error
	acquireCount    int
	released        []string
	leasesByKey     map[string]backend.LeaseRecord
	presenceRecords map[string]backend.LeaseRecord
}

func newFailingCompositeDriver(failAt int, failErr error) *failingCompositeDriver {
	return &failingCompositeDriver{
		failAt:          failAt,
		failErr:         failErr,
		leasesByKey:     make(map[string]backend.LeaseRecord),
		presenceRecords: make(map[string]backend.LeaseRecord),
	}
}

func (d *failingCompositeDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.acquireCount++
	if d.failAt > 0 && d.acquireCount == d.failAt {
		return backend.LeaseRecord{}, d.failErr
	}

	now := time.Now()
	lease := backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: append([]string(nil), req.ResourceKeys...),
		OwnerID:      req.OwnerID,
		LeaseTTL:     req.LeaseTTL,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(req.LeaseTTL),
	}
	if len(req.ResourceKeys) > 0 {
		key := req.ResourceKeys[0]
		d.leasesByKey[key] = lease
		d.presenceRecords[key] = lease
	}
	return lease, nil
}

func (d *failingCompositeDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	return lease, nil
}

func (d *failingCompositeDriver) AcquireWithLineage(ctx context.Context, req backend.LineageAcquireRequest) (backend.LeaseRecord, error) {
	return d.Acquire(ctx, backend.AcquireRequest{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{req.ResourceKey},
		OwnerID:      req.OwnerID,
		LeaseTTL:     req.LeaseTTL,
	})
}

func (d *failingCompositeDriver) RenewWithLineage(
	ctx context.Context,
	lease backend.LeaseRecord,
	lineage backend.LineageLeaseMeta,
) (backend.LeaseRecord, backend.LineageLeaseMeta, error) {
	renewed, err := d.Renew(ctx, lease)
	if err != nil {
		return backend.LeaseRecord{}, backend.LineageLeaseMeta{}, err
	}
	return renewed, lineage, nil
}

func (d *failingCompositeDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(lease.ResourceKeys) == 0 {
		return nil
	}

	key := lease.ResourceKeys[0]
	d.released = append(d.released, key)
	delete(d.leasesByKey, key)
	delete(d.presenceRecords, key)
	return nil
}

func (d *failingCompositeDriver) ReleaseWithLineage(
	ctx context.Context,
	lease backend.LeaseRecord,
	lineage backend.LineageLeaseMeta,
) error {
	return d.Release(ctx, lease)
}

func (d *failingCompositeDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	record := backend.PresenceRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: append([]string(nil), req.ResourceKeys...),
	}
	if len(req.ResourceKeys) == 0 {
		return record, nil
	}
	if lease, ok := d.presenceRecords[req.ResourceKeys[0]]; ok {
		record.Present = true
		record.Lease = lease
		record.DefinitionID = lease.DefinitionID
		record.ResourceKeys = append([]string(nil), lease.ResourceKeys...)
	}
	return record, nil
}

func (d *failingCompositeDriver) Ping(ctx context.Context) error {
	return nil
}

func (d *failingCompositeDriver) acquireAttempts() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.acquireCount
}

func (d *failingCompositeDriver) releasedKeys() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.released...)
}

func TestExecuteCompositeExclusiveEmitsBridgeEvents(t *testing.T) {
	reg := newCompositeRegistry(t)
	driver := testkit.NewMemoryDriver()
	bridge := &bridgeStub{}
	mgr, err := NewManager(reg, driver, nil, WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteCompositeExclusive(context.Background(), compositeRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteCompositeExclusive returned error: %v", err)
	}

	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	// Two members: acquire started + succeeded for each.
	if bridge.acquireStarted != 2 {
		t.Fatalf("expected 2 acquire started events, got %d", bridge.acquireStarted)
	}
	if bridge.acquireSucceeded != 2 {
		t.Fatalf("expected 2 acquire succeeded events, got %d", bridge.acquireSucceeded)
	}
	// Two members released in reverse order.
	if bridge.released != 2 {
		t.Fatalf("expected 2 released events, got %d", bridge.released)
	}
	if bridge.acquireFailed != 0 {
		t.Fatalf("expected 0 acquire failed events, got %d", bridge.acquireFailed)
	}
}
