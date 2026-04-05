package workers

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/backend/memory"
	memstore "github.com/tuanuet/lockman/idempotency/memory"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/registry"
	"github.com/tuanuet/lockman/observe"
)

func TestNewManagerRejectsInvalidRegistry(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "BrokenAsyncLock",
		Kind:          backend.KindParent,
		Resource:      "broken",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionAsync,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err := NewManager(reg, memory.NewMemoryDriver(), memstore.NewStore())
	if !errors.Is(err, lockerrors.ErrRegistryViolation) {
		t.Fatalf("expected registry violation, got %v", err)
	}
}

func TestNewManagerRejectsLineageRegistryWithoutLineageDriver(t *testing.T) {
	reg := workerRegistryWithLineageChain(t)

	_, err := NewManager(reg, exactOnlyDriverStub{inner: memory.NewMemoryDriver()}, memstore.NewStore())
	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation for missing lineage driver capability, got %v", err)
	}
}

func TestNewManagerRejectsStrictAsyncRegistryWithoutStrictDriver(t *testing.T) {
	reg := strictWorkerRegistryForTest(t, definitions.ExecutionAsync)

	_, err := NewManager(reg, exactOnlyDriverStub{inner: memory.NewMemoryDriver()}, memstore.NewStore())
	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation for missing strict driver capability, got %v", err)
	}
}

func TestNewManagerRejectsStrictBothRegistryWithoutStrictDriver(t *testing.T) {
	reg := strictWorkerRegistryForTest(t, definitions.ExecutionBoth)

	_, err := NewManager(reg, exactOnlyDriverStub{inner: memory.NewMemoryDriver()}, memstore.NewStore())
	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation for missing strict driver capability, got %v", err)
	}
}

func TestNewManagerAllowsStrictSyncOnlyRegistryWithoutStrictDriver(t *testing.T) {
	reg := strictWorkerRegistryForTest(t, definitions.ExecutionSync)

	mgr, err := NewManager(reg, exactOnlyDriverStub{inner: memory.NewMemoryDriver()}, nil)
	if err != nil {
		t.Fatalf("expected worker manager to allow sync-only strict definitions, got %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestNewManagerRequiresIdempotencyStoreWhenAsyncDefinitionNeedsIt(t *testing.T) {
	reg := newWorkerRegistryForTest(t, true)

	_, err := NewManager(reg, memory.NewMemoryDriver(), nil)
	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation for missing idempotency store, got %v", err)
	}
}

func TestNewManagerAllowsNilIdempotencyStoreWhenNotRequired(t *testing.T) {
	reg := newWorkerRegistryForTest(t, false)

	mgr, err := NewManager(reg, memory.NewMemoryDriver(), nil)
	if err != nil {
		t.Fatalf("expected manager creation to succeed without idempotency store, got %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestShutdownStopsNewClaims(t *testing.T) {
	mgr := newWorkerManagerForTest(t)

	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	err := mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
		return nil
	})
	if !errors.Is(err, lockerrors.ErrWorkerShuttingDown) {
		t.Fatalf("expected worker shutting down error, got %v", err)
	}
}

func TestShutdownWaitsForInFlightClaimExecutionToDrain(t *testing.T) {
	mgr := newWorkerManagerForTest(t)

	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	execErrCh := make(chan error, 1)
	go func() {
		execErrCh <- mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
			close(callbackEntered)
			<-releaseCallback
			return nil
		})
	}()

	<-callbackEntered

	shutdownErrCh := make(chan error, 1)
	go func() {
		shutdownErrCh <- mgr.Shutdown(context.Background())
	}()

	select {
	case err := <-shutdownErrCh:
		t.Fatalf("Shutdown returned before in-flight claim drained: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	close(releaseCallback)

	if err := <-shutdownErrCh; err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
	if err := <-execErrCh; err != nil {
		t.Fatalf("ExecuteClaimed returned error: %v", err)
	}
}

func TestShutdownReturnsContextErrorWhenInFlightDoesNotDrain(t *testing.T) {
	mgr := newWorkerManagerForTest(t)

	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	execErrCh := make(chan error, 1)
	go func() {
		execErrCh <- mgr.ExecuteClaimed(context.Background(), messageClaimRequest(), func(ctx context.Context, claim definitions.ClaimContext) error {
			close(callbackEntered)
			<-releaseCallback
			return nil
		})
	}()

	<-callbackEntered

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := mgr.Shutdown(shutdownCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}

	close(releaseCallback)
	if err := <-execErrCh; err != nil {
		t.Fatalf("ExecuteClaimed returned error: %v", err)
	}
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown should succeed after drain, got %v", err)
	}
}

func TestShutdownIsIdempotent(t *testing.T) {
	mgr := newWorkerManagerForTest(t)

	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown returned error: %v", err)
	}
	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown should be idempotent, got %v", err)
	}
}

func TestShutdownWaitsForInFlightClaimWithoutPrematureRenewalCancellation(t *testing.T) {
	reg := newWorkerRegistryForTest(t, true)
	store := memstore.NewStore()
	driver := &renewObserveDriver{base: memory.NewMemoryDriver()}

	mgr, err := NewManager(reg, driver, store)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	req := messageClaimRequest()
	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	execErrCh := make(chan error, 1)
	go func() {
		execErrCh <- mgr.ExecuteClaimed(context.Background(), req, func(ctx context.Context, claim definitions.ClaimContext) error {
			close(callbackEntered)
			<-releaseCallback
			return nil
		})
	}()

	<-callbackEntered

	shutdownErrCh := make(chan error, 1)
	go func() {
		shutdownErrCh <- mgr.Shutdown(context.Background())
	}()

	time.Sleep(140 * time.Millisecond)

	def := reg.MustGet(req.DefinitionID)
	resourceKey, err := def.KeyBuilder.Build(req.KeyInput)
	if err != nil {
		t.Fatalf("key build failed: %v", err)
	}

	presence, err := driver.CheckPresence(context.Background(), backend.PresenceRequest{
		DefinitionID: req.DefinitionID,
		ResourceKeys: []string{resourceKey},
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}
	if !presence.Present {
		t.Fatal("expected lease to remain present while shutdown drains in-flight claim")
	}
	if driver.renewCount() == 0 {
		t.Fatal("expected renewal to continue while shutdown waits for in-flight claim")
	}

	select {
	case err := <-shutdownErrCh:
		t.Fatalf("Shutdown returned before in-flight callback drained: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	close(releaseCallback)
	if err := <-execErrCh; err != nil {
		t.Fatalf("ExecuteClaimed returned error: %v", err)
	}
	if err := <-shutdownErrCh; err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
}

type renewObserveDriver struct {
	base       *memory.MemoryDriver
	renewCalls atomic.Int32
}

func (d *renewObserveDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	return d.base.Acquire(ctx, req)
}

func (d *renewObserveDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	d.renewCalls.Add(1)
	return d.base.Renew(ctx, lease)
}

func (d *renewObserveDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return d.base.Release(ctx, lease)
}

func (d *renewObserveDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return d.base.CheckPresence(ctx, req)
}

func (d *renewObserveDriver) Ping(ctx context.Context) error {
	return d.base.Ping(ctx)
}

func (d *renewObserveDriver) renewCount() int {
	return int(d.renewCalls.Load())
}

type exactOnlyDriverStub struct {
	inner backend.Driver
}

func (d exactOnlyDriverStub) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	return d.inner.Acquire(ctx, req)
}

func (d exactOnlyDriverStub) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	return d.inner.Renew(ctx, lease)
}

func (d exactOnlyDriverStub) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return d.inner.Release(ctx, lease)
}

func (d exactOnlyDriverStub) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return d.inner.CheckPresence(ctx, req)
}

func (d exactOnlyDriverStub) Ping(ctx context.Context) error {
	return d.inner.Ping(ctx)
}

func workerRegistryWithLineageChain(t *testing.T) *registry.Registry {
	t.Helper()

	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionAsync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register parent failed: %v", err)
	}

	if err := reg.Register(definitions.LockDefinition{
		ID:            "ItemLock",
		Kind:          backend.KindChild,
		Resource:      "item",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionAsync,
		LeaseTTL:      30 * time.Second,
		ParentRef:     "OrderLock",
		OverlapPolicy: definitions.OverlapReject,
		KeyBuilder: definitions.MustTemplateKeyBuilder(
			"order:{order_id}:item:{item_id}",
			[]string{"order_id", "item_id"},
		),
	}); err != nil {
		t.Fatalf("register child failed: %v", err)
	}

	return reg
}

func strictWorkerRegistryForTest(t *testing.T, kind definitions.ExecutionKind) *registry.Registry {
	t.Helper()

	reg := registry.New()
	def := definitions.LockDefinition{
		ID:                   "StrictMessageLock",
		Kind:                 backend.KindParent,
		Resource:             "message",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        kind,
		LeaseTTL:             30 * time.Second,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("message:{message_id}", []string{"message_id"}),
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		IdempotencyRequired:  kind == definitions.ExecutionAsync || kind == definitions.ExecutionBoth,
	}
	if err := reg.Register(def); err != nil {
		t.Fatalf("register strict definition failed: %v", err)
	}
	return reg
}

func TestNewManagerStillCompilesWithExistingThreeParamShape(t *testing.T) {
	reg := newWorkerRegistryForTest(t, false)
	mgr, err := NewManager(reg, memory.NewMemoryDriver(), nil)
	if err != nil {
		t.Fatalf("NewManager with three params returned error: %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestNewManagerAcceptsVariadicBridgeOption(t *testing.T) {
	reg := newWorkerRegistryForTest(t, false)
	var events []observe.Event
	bridge := workerTestBridge(func(event observe.Event) {
		events = append(events, event)
	})
	mgr, err := NewManager(reg, memory.NewMemoryDriver(), nil, WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager with bridge option returned error: %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestShutdownEmitsObservabilityEvents(t *testing.T) {
	reg := newWorkerRegistryForTest(t, false)
	var events []observe.Event
	bridge := workerTestBridge(func(event observe.Event) {
		events = append(events, event)
	})
	mgr, err := NewManager(reg, memory.NewMemoryDriver(), nil, WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	if err := mgr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	if !hasEventKind(events, observe.EventShutdownStarted) {
		t.Fatal("expected shutdown_started event")
	}
	if !hasEventKind(events, observe.EventShutdownCompleted) {
		t.Fatal("expected shutdown_completed event")
	}
}

func hasEventKind(events []observe.Event, kind observe.EventKind) bool {
	for _, e := range events {
		if e.Kind == kind {
			return true
		}
	}
	return false
}

type workerTestBridge func(observe.Event)

func (f workerTestBridge) PublishWorkerAcquireStarted(e observe.Event) {
	e.Kind = observe.EventAcquireStarted
	f(e)
}
func (f workerTestBridge) PublishWorkerAcquireSucceeded(e observe.Event) {
	e.Kind = observe.EventAcquireSucceeded
	f(e)
}
func (f workerTestBridge) PublishWorkerAcquireFailed(e observe.Event, err error) {
	e.Kind = observe.EventAcquireFailed
	e.Error = err
	f(e)
}
func (f workerTestBridge) PublishWorkerReleased(e observe.Event) {
	e.Kind = observe.EventReleased
	f(e)
}
func (f workerTestBridge) PublishWorkerOverlap(e observe.Event) {
	e.Kind = observe.EventOverlap
	f(e)
}
func (f workerTestBridge) PublishWorkerRenewalSucceeded(e observe.Event) {
	e.Kind = observe.EventRenewalSucceeded
	f(e)
}
func (f workerTestBridge) PublishWorkerLeaseLost(e observe.Event) {
	e.Kind = observe.EventLeaseLost
	f(e)
}
func (f workerTestBridge) PublishWorkerShutdownStarted() {
	f(observe.Event{Kind: observe.EventShutdownStarted})
}
func (f workerTestBridge) PublishWorkerShutdownCompleted() {
	f(observe.Event{Kind: observe.EventShutdownCompleted})
}
