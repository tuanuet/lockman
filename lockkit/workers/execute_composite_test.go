package workers

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/backend/memory"
	"github.com/tuanuet/lockman/idempotency"
	memstore "github.com/tuanuet/lockman/idempotency/memory"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/internal/policy"
	"github.com/tuanuet/lockman/lockkit/registry"
	"github.com/tuanuet/lockman/observe"
)

type compositeWorkerManagerHarness struct {
	*Manager
	testStore  *memstore.Store
	testDriver backend.Driver
}

func TestExecuteCompositeClaimedPopulatesResourceKeysInCanonicalOrder(t *testing.T) {
	mgr := newCompositeWorkerManagerForTest(t)

	var gotKeys []string
	err := mgr.ExecuteCompositeClaimed(context.Background(), compositeClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		gotKeys = append(gotKeys, claim.ResourceKeys...)
		if claim.IdempotencyKey == "" {
			t.Fatal("expected claim idempotency key")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteCompositeClaimed returned error: %v", err)
	}

	if len(gotKeys) != 2 {
		t.Fatalf("expected 2 resource keys, got %d", len(gotKeys))
	}
	if gotKeys[0] != "account:acct-123" || gotKeys[1] != "ledger:ledger-456" {
		t.Fatalf("expected canonical resource keys [account:acct-123 ledger:ledger-456], got %v", gotKeys)
	}
}

func TestExecuteCompositeClaimedRollsBackOnPartialAcquireFailure(t *testing.T) {
	reg := newRollbackCompositeWorkerRegistry(t)
	store := memstore.NewStore()
	driver := newFailingCompositeClaimDriver(2, backend.ErrLeaseAlreadyHeld)
	mgr, err := NewManager(reg, driver, store)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteCompositeClaimed(context.Background(), rollbackCompositeClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		t.Fatal("callback should not run when composite acquire fails")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrLockBusy) {
		t.Fatalf("expected composite acquire failure, got %v", err)
	}

	assertNotHeld(t, driver, "AccountA", "account:a")
	assertNotHeld(t, driver, "AccountB", "account:b")
}

func TestExecuteCompositeClaimedRejectsOverlapBeforeAcquire(t *testing.T) {
	reg := newOverlapCompositeWorkerRegistry(t)
	store := memstore.NewStore()
	driver := newFailingCompositeClaimDriver(0, nil)
	mgr, err := NewManager(reg, driver, store)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	called := false
	err = mgr.ExecuteCompositeClaimed(context.Background(), overlapCompositeClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		called = true
		return nil
	})
	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation for overlap rejection, got %v", err)
	}
	if called {
		t.Fatal("callback should not run when overlap is rejected")
	}
	if attempts := driver.acquireAttempts(); attempts != 0 {
		t.Fatalf("expected overlap rejection before acquire attempts, got %d", attempts)
	}
}

func TestExecuteCompositeClaimedUsesLineageDriverForLineageMembers(t *testing.T) {
	driver := memory.NewMemoryDriver()
	reg := workerRegistryWithCompositeLineageMembers(t)
	holder := newWorkerManagerWithDriver(t, reg, driver)
	compositeMgr := newWorkerManagerWithDriver(t, reg, driver)

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- holder.ExecuteClaimed(context.Background(), parentMessageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	err := compositeMgr.ExecuteCompositeClaimed(context.Background(), compositeChildMemberClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		t.Fatal("composite callback should not run while parent is held")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection, got %v", err)
	}
	if got := policy.OutcomeFromError(err); got != policy.OutcomeRetry {
		t.Fatalf("expected retry outcome, got %q", got)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("holder ExecuteClaimed returned error: %v", err)
	}
}

func TestExecuteCompositeClaimedSameProcessReentrantRejected(t *testing.T) {
	mgr := newCompositeWorkerManagerForTest(t)

	req := compositeClaimRequest()
	entered := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- mgr.ExecuteCompositeClaimed(context.Background(), req, func(ctx context.Context, claim definitions.ClaimContext) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	err := mgr.ExecuteCompositeClaimed(context.Background(), req, func(ctx context.Context, claim definitions.ClaimContext) error {
		t.Fatal("callback should not run for reentrant composite claim")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrReentrantAcquire) {
		t.Fatalf("expected reentrant error, got %v", err)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first ExecuteCompositeClaimed returned error: %v", err)
	}
}

func TestExecuteCompositeClaimedPersistsIdempotencyBeforeAck(t *testing.T) {
	mgr := newCompositeWorkerManagerForTest(t)

	err := mgr.ExecuteCompositeClaimed(context.Background(), compositeClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteCompositeClaimed returned error: %v", err)
	}

	record, err := mgr.testStore.Get(context.Background(), "transfer:123")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if record.Status != idempotency.StatusCompleted {
		t.Fatalf("expected completed status, got %q", record.Status)
	}
}

func TestExecuteCompositeClaimedUsesMaxMemberLeaseTTLForIdempotency(t *testing.T) {
	shortTTL := 20 * time.Second
	longTTL := 45 * time.Second

	reg := newCompositeWorkerRegistryWithTTLs(t, shortTTL, longTTL)
	store := memstore.NewStore()
	mgr, err := NewManager(reg, memory.NewMemoryDriver(), store)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteCompositeClaimed(context.Background(), compositeClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		return lockerrors.ErrLockBusy
	})
	if !errors.Is(err, lockerrors.ErrLockBusy) {
		t.Fatalf("expected retry-mapped callback error, got %v", err)
	}

	record, err := store.Get(context.Background(), "transfer:123")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if record.Status != idempotency.StatusInProgress {
		t.Fatalf("expected in-progress idempotency status for retry path, got %q", record.Status)
	}

	remainingTTL := time.Until(record.ExpiresAt)
	minExpected := inProgressTTL(longTTL) - 5*time.Second
	if remainingTTL < minExpected {
		t.Fatalf("expected in-progress ttl based on longest member lease (remaining=%s, want >= %s)", remainingTTL, minExpected)
	}
}

func TestExecuteCompositeClaimedCancelsContextWhenAnyMemberRenewalFails(t *testing.T) {
	reg := newCompositeWorkerRegistryForTest(t)
	store := memstore.NewStore()
	driver := &multiMemberRenewFailDriver{
		base:            memory.NewMemoryDriver(),
		failResourceKey: "ledger:ledger-456",
	}
	mgr, err := NewManager(reg, driver, store)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	callbackCanceled := make(chan struct{})
	err = mgr.ExecuteCompositeClaimed(context.Background(), compositeClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		<-ctx.Done()
		close(callbackCanceled)
		return ctx.Err()
	})
	if !errors.Is(err, lockerrors.ErrLeaseLost) {
		t.Fatalf("expected lease lost after member renewal failure, got %v", err)
	}
	select {
	case <-callbackCanceled:
	default:
		t.Fatal("expected callback context cancellation on member renewal failure")
	}

	record, err := store.Get(context.Background(), "transfer:123")
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if record.Status != idempotency.StatusInProgress {
		t.Fatalf("expected in-progress status for retry path, got %q", record.Status)
	}
}

func TestExecuteCompositeClaimedRejectsWhenShuttingDown(t *testing.T) {
	mgr := newCompositeWorkerManagerForTest(t)
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	err := mgr.ExecuteCompositeClaimed(context.Background(), compositeClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		t.Fatal("callback should not run during shutdown")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrWorkerShuttingDown) {
		t.Fatalf("expected worker shutting down error, got %v", err)
	}
}

func newCompositeWorkerManagerForTest(t *testing.T) compositeWorkerManagerHarness {
	t.Helper()

	reg := newCompositeWorkerRegistryForTest(t)
	store := memstore.NewStore()
	driver := memory.NewMemoryDriver()
	mgr, err := NewManager(reg, driver, store)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	return compositeWorkerManagerHarness{
		Manager:    mgr,
		testStore:  store,
		testDriver: driver,
	}
}

func newCompositeWorkerRegistryForTest(t *testing.T) *registry.Registry {
	return newCompositeWorkerRegistryWithTTLs(t, 90*time.Millisecond, 90*time.Millisecond)
}

func newCompositeWorkerRegistryWithTTLs(t *testing.T, accountTTL, ledgerTTL time.Duration) *registry.Registry {
	t.Helper()

	reg := registry.New()
	register := func(def definitions.LockDefinition) {
		if err := reg.Register(def); err != nil {
			t.Fatalf("register %s failed: %v", def.ID, err)
		}
	}

	register(definitions.LockDefinition{
		ID:                  "LedgerMember",
		Kind:                definitions.KindParent,
		Resource:            "ledger",
		Mode:                definitions.ModeStandard,
		ExecutionKind:       definitions.ExecutionAsync,
		LeaseTTL:            ledgerTTL,
		Rank:                20,
		IdempotencyRequired: true,
		KeyBuilder:          definitions.MustTemplateKeyBuilder("ledger:{ledger_id}", []string{"ledger_id"}),
	})
	register(definitions.LockDefinition{
		ID:                  "AccountMember",
		Kind:                definitions.KindParent,
		Resource:            "account",
		Mode:                definitions.ModeStandard,
		ExecutionKind:       definitions.ExecutionAsync,
		LeaseTTL:            accountTTL,
		Rank:                10,
		IdempotencyRequired: true,
		KeyBuilder:          definitions.MustTemplateKeyBuilder("account:{account_id}", []string{"account_id"}),
	})

	if err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:               "TransferComposite",
		Members:          []string{"LedgerMember", "AccountMember"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   2,
		ExecutionKind:    definitions.ExecutionAsync,
	}); err != nil {
		t.Fatalf("register TransferComposite failed: %v", err)
	}

	return reg
}

func compositeClaimRequest() definitions.CompositeClaimRequest {
	return definitions.CompositeClaimRequest{
		DefinitionID:   "TransferComposite",
		IdempotencyKey: "transfer:123",
		MemberInputs: []map[string]string{
			{
				"ledger_id": "ledger-456",
			},
			{
				"account_id": "acct-123",
			},
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "worker-a",
			MessageID:     "123",
			Attempt:       1,
			ConsumerGroup: "payments",
			HandlerName:   "TransferFunds",
		},
	}
}

func workerRegistryWithCompositeLineageMembers(t *testing.T) *registry.Registry {
	t.Helper()

	reg := workerRegistryWithLineageChain(t)
	if err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:               "ChildOnlyComposite",
		Members:          []string{"ItemLock"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   1,
		ExecutionKind:    definitions.ExecutionAsync,
	}); err != nil {
		t.Fatalf("register ChildOnlyComposite failed: %v", err)
	}

	return reg
}

func compositeChildMemberClaimRequest() definitions.CompositeClaimRequest {
	return definitions.CompositeClaimRequest{
		DefinitionID:   "ChildOnlyComposite",
		IdempotencyKey: "order:123:item:line-1",
		MemberInputs: []map[string]string{
			{
				"order_id": "123",
				"item_id":  "line-1",
			},
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "worker-composite",
			MessageID:     "line-1",
			Attempt:       1,
			ConsumerGroup: "payments",
			HandlerName:   "HandleCompositeItem",
		},
	}
}

func newRollbackCompositeWorkerRegistry(t *testing.T) *registry.Registry {
	t.Helper()

	reg := registry.New()
	register := func(def definitions.LockDefinition) {
		if err := reg.Register(def); err != nil {
			t.Fatalf("register %s failed: %v", def.ID, err)
		}
	}

	register(definitions.LockDefinition{
		ID:                  "AccountA",
		Kind:                definitions.KindParent,
		Resource:            "account",
		Mode:                definitions.ModeStandard,
		ExecutionKind:       definitions.ExecutionAsync,
		LeaseTTL:            90 * time.Millisecond,
		Rank:                1,
		IdempotencyRequired: true,
		KeyBuilder:          definitions.MustTemplateKeyBuilder("account:{id}", []string{"id"}),
	})
	register(definitions.LockDefinition{
		ID:                  "AccountB",
		Kind:                definitions.KindParent,
		Resource:            "account",
		Mode:                definitions.ModeStandard,
		ExecutionKind:       definitions.ExecutionAsync,
		LeaseTTL:            90 * time.Millisecond,
		Rank:                2,
		IdempotencyRequired: true,
		KeyBuilder:          definitions.MustTemplateKeyBuilder("account:{id}", []string{"id"}),
	})

	if err := reg.RegisterComposite(definitions.CompositeDefinition{
		ID:               "RollbackComposite",
		Members:          []string{"AccountA", "AccountB"},
		OrderingPolicy:   definitions.OrderingCanonical,
		AcquirePolicy:    definitions.AcquireAllOrNothing,
		EscalationPolicy: definitions.EscalationReject,
		ModeResolution:   definitions.ModeResolutionHomogeneous,
		MaxMemberCount:   2,
		ExecutionKind:    definitions.ExecutionAsync,
	}); err != nil {
		t.Fatalf("register RollbackComposite failed: %v", err)
	}

	return reg
}

func rollbackCompositeClaimRequest() definitions.CompositeClaimRequest {
	return definitions.CompositeClaimRequest{
		DefinitionID:   "RollbackComposite",
		IdempotencyKey: "rollback:123",
		MemberInputs: []map[string]string{
			{
				"id": "a",
			},
			{
				"id": "b",
			},
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "worker-a",
			MessageID:     "123",
			Attempt:       1,
			ConsumerGroup: "payments",
			HandlerName:   "TransferFunds",
		},
	}
}

func newOverlapCompositeWorkerRegistry(t *testing.T) *registry.Registry {
	t.Helper()

	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:                  "OrderParent",
		Kind:                definitions.KindParent,
		Resource:            "order",
		Mode:                definitions.ModeStandard,
		ExecutionKind:       definitions.ExecutionAsync,
		LeaseTTL:            90 * time.Millisecond,
		Rank:                10,
		IdempotencyRequired: true,
		KeyBuilder:          definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register OrderParent failed: %v", err)
	}
	if err := reg.Register(definitions.LockDefinition{
		ID:                  "OrderChild",
		Kind:                definitions.KindChild,
		ParentRef:           "OrderParent",
		OverlapPolicy:       definitions.OverlapReject,
		Resource:            "order_line",
		Mode:                definitions.ModeStandard,
		ExecutionKind:       definitions.ExecutionAsync,
		LeaseTTL:            90 * time.Millisecond,
		Rank:                20,
		IdempotencyRequired: true,
		KeyBuilder:          definitions.MustTemplateKeyBuilder("order:{order_id}:{item_id}", []string{"order_id", "item_id"}),
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
		ExecutionKind:    definitions.ExecutionAsync,
	}); err != nil {
		t.Fatalf("register OrderComposite failed: %v", err)
	}

	return reg
}

func overlapCompositeClaimRequest() definitions.CompositeClaimRequest {
	return definitions.CompositeClaimRequest{
		DefinitionID:   "OrderComposite",
		IdempotencyKey: "order:123",
		MemberInputs: []map[string]string{
			{
				"order_id": "123",
			},
			{
				"order_id": "123",
				"item_id":  "line-1",
			},
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID:       "worker-a",
			MessageID:     "123",
			Attempt:       1,
			ConsumerGroup: "payments",
			HandlerName:   "ProcessOrder",
		},
	}
}

func assertNotHeld(t *testing.T, driver backend.Driver, definitionID string, key string) {
	t.Helper()

	presence, err := driver.CheckPresence(context.Background(), backend.PresenceRequest{
		DefinitionID: definitionID,
		ResourceKeys: []string{key},
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}
	if presence.Present {
		t.Fatalf("expected %q to be released, but it is still held", key)
	}
}

type failingCompositeClaimDriver struct {
	base         *memory.MemoryDriver
	failAt       int32
	failErr      error
	acquireCount atomic.Int32
}

func newFailingCompositeClaimDriver(failAt int, failErr error) *failingCompositeClaimDriver {
	return &failingCompositeClaimDriver{
		base:    memory.NewMemoryDriver(),
		failAt:  int32(failAt),
		failErr: failErr,
	}
}

func (d *failingCompositeClaimDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	attempt := d.acquireCount.Add(1)
	if d.failAt > 0 && attempt == d.failAt {
		return backend.LeaseRecord{}, d.failErr
	}
	return d.base.Acquire(ctx, req)
}

func (d *failingCompositeClaimDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	return d.base.Renew(ctx, lease)
}

func (d *failingCompositeClaimDriver) AcquireWithLineage(ctx context.Context, req backend.LineageAcquireRequest) (backend.LeaseRecord, error) {
	attempt := d.acquireCount.Add(1)
	if d.failAt > 0 && attempt == d.failAt {
		return backend.LeaseRecord{}, d.failErr
	}
	return d.base.AcquireWithLineage(ctx, req)
}

func (d *failingCompositeClaimDriver) RenewWithLineage(
	ctx context.Context,
	lease backend.LeaseRecord,
	lineage backend.LineageLeaseMeta,
) (backend.LeaseRecord, backend.LineageLeaseMeta, error) {
	return d.base.RenewWithLineage(ctx, lease, lineage)
}

func (d *failingCompositeClaimDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return d.base.Release(ctx, lease)
}

func (d *failingCompositeClaimDriver) ReleaseWithLineage(
	ctx context.Context,
	lease backend.LeaseRecord,
	lineage backend.LineageLeaseMeta,
) error {
	return d.base.ReleaseWithLineage(ctx, lease, lineage)
}

func (d *failingCompositeClaimDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return d.base.CheckPresence(ctx, req)
}

func (d *failingCompositeClaimDriver) Ping(ctx context.Context) error {
	return d.base.Ping(ctx)
}

func (d *failingCompositeClaimDriver) acquireAttempts() int {
	return int(d.acquireCount.Load())
}

type multiMemberRenewFailDriver struct {
	base            *memory.MemoryDriver
	failResourceKey string
	failed          atomic.Bool
}

func (d *multiMemberRenewFailDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	return d.base.Acquire(ctx, req)
}

func (d *multiMemberRenewFailDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	if len(lease.ResourceKeys) == 1 && lease.ResourceKeys[0] == d.failResourceKey && d.failed.CompareAndSwap(false, true) {
		return backend.LeaseRecord{}, backend.ErrLeaseExpired
	}
	return d.base.Renew(ctx, lease)
}

func (d *multiMemberRenewFailDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return d.base.Release(ctx, lease)
}

func (d *multiMemberRenewFailDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return d.base.CheckPresence(ctx, req)
}

func (d *multiMemberRenewFailDriver) Ping(ctx context.Context) error {
	return d.base.Ping(ctx)
}

func TestExecuteCompositeClaimedEmitsMemberAcquireEvents(t *testing.T) {
	reg := newCompositeWorkerRegistryForTest(t)
	var events []observe.Event
	bridge := workerTestBridge(func(event observe.Event) {
		events = append(events, event)
	})
	mgr, err := NewManager(reg, memory.NewMemoryDriver(), memstore.NewStore(), WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteCompositeClaimed(context.Background(), compositeClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteCompositeClaimed returned error: %v", err)
	}

	acquireStarted := 0
	acquireSucceeded := 0
	released := 0
	for _, e := range events {
		switch e.Kind {
		case observe.EventAcquireStarted:
			acquireStarted++
		case observe.EventAcquireSucceeded:
			acquireSucceeded++
		case observe.EventReleased:
			released++
		}
	}
	if acquireStarted < 2 {
		t.Fatalf("expected at least 2 acquire_started events for composite members, got %d", acquireStarted)
	}
	if acquireSucceeded < 2 {
		t.Fatalf("expected at least 2 acquire_succeeded events for composite members, got %d", acquireSucceeded)
	}
	if released < 2 {
		t.Fatalf("expected at least 2 released events for composite members, got %d", released)
	}
}

func TestExecuteCompositeClaimedEmitsLeaseLostOnMemberRenewalFailure(t *testing.T) {
	reg := newCompositeWorkerRegistryForTest(t)
	store := memstore.NewStore()
	driver := &multiMemberRenewFailDriver{
		base:            memory.NewMemoryDriver(),
		failResourceKey: "ledger:ledger-456",
	}
	var events []observe.Event
	bridge := workerTestBridge(func(event observe.Event) {
		events = append(events, event)
	})
	mgr, err := NewManager(reg, driver, store, WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteCompositeClaimed(context.Background(), compositeClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, lockerrors.ErrLeaseLost) {
		t.Fatalf("expected lease lost error, got %v", err)
	}
	if !hasEventKind(events, observe.EventLeaseLost) {
		t.Fatal("expected lease_lost event for composite member renewal failure")
	}
}
