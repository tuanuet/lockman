package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/backend/memory"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockobserve "github.com/tuanuet/lockman/lockkit/observe"
	"github.com/tuanuet/lockman/lockkit/registry"
	"github.com/tuanuet/lockman/observe"

	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
)

func TestExecuteExclusiveRunsCallbackWhenLockAcquired(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	called := false
	err = mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			ServiceName: "svc",
			InstanceID:  "one",
			HandlerName: "UpdateOrder",
			OwnerID:     "svc:one",
		},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		called = true
		if lease.ResourceKey != "order:123" {
			t.Fatalf("unexpected resource key: %q", lease.ResourceKey)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("ExecuteExclusive returned error: %v", err)
	}
	if !called {
		t.Fatal("expected callback to run")
	}
}

func TestExecuteExclusiveRunsCallbackWithDirectResourceKey(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	called := false
	err = mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		ResourceKey:  "order:123",
		Ownership: definitions.OwnershipMeta{
			ServiceName: "svc",
			InstanceID:  "one",
			HandlerName: "UpdateOrder",
			OwnerID:     "svc:one",
		},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		called = true
		if lease.ResourceKey != "order:123" {
			t.Fatalf("unexpected resource key: %q", lease.ResourceKey)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("ExecuteExclusive returned error: %v", err)
	}
	if !called {
		t.Fatal("expected callback to run")
	}
}

func TestExecuteExclusiveRejectsReentrantAcquire(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	req := definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			ServiceName: "svc",
			InstanceID:  "one",
			HandlerName: "UpdateOrder",
			OwnerID:     "svc:one",
		},
	}

	err = mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		return mgr.ExecuteExclusive(ctx, req, func(ctx context.Context, nested definitions.LeaseContext) error {
			return nil
		})
	})

	if !errors.Is(err, lockerrors.ErrReentrantAcquire) {
		t.Fatalf("expected reentrant acquire error, got %v", err)
	}
}

func TestExecuteExclusiveGuardHandlesColonCharacters(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "Order:Lock",
		Kind:          backend.KindParent,
		Resource:      "order:group",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	req := definitions.SyncLockRequest{
		DefinitionID: "Order:Lock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			ServiceName: "svc",
			InstanceID:  "one",
			HandlerName: "UpdateOrder",
			OwnerID:     "svc:one",
		},
	}

	err = mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		return mgr.ExecuteExclusive(ctx, req, func(ctx context.Context, nested definitions.LeaseContext) error {
			return nil
		})
	})

	if !errors.Is(err, lockerrors.ErrReentrantAcquire) {
		t.Fatalf("expected reentrant acquire error with colon id, got %v", err)
	}
}

func TestExecuteExclusiveUnknownDefinitionReturnsError(t *testing.T) {
	reg := registry.New()
	mgr, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
		DefinitionID: "MissingLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		return nil
	})

	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation for missing definition, got %v", err)
	}
}

func TestNewManagerRejectsInvalidRegistry(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "BrokenLock",
		Kind:          backend.KindParent,
		Resource:      "broken",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	_, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder())
	if err == nil {
		t.Fatal("expected invalid registry rejection")
	}
}

func TestRuntimeManagerRejectsLineageRegistryWithoutLineageDriver(t *testing.T) {
	reg := registryWithLineageChain(t)
	_, err := NewManager(reg, exactOnlyDriverStub{inner: memory.NewMemoryDriver()}, lockobserve.NewNoopRecorder())
	if err == nil || !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected manager capability rejection, got %v", err)
	}
}

func TestRuntimeManagerRejectsStrictSyncRegistryWithoutStrictDriver(t *testing.T) {
	reg := strictRuntimeRegistryForTest(t, definitions.ExecutionSync)

	_, err := NewManager(reg, exactOnlyDriverStub{inner: memory.NewMemoryDriver()}, lockobserve.NewNoopRecorder())
	if err == nil || !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation for missing strict driver capability, got %v", err)
	}
}

func TestRuntimeManagerRejectsStrictBothRegistryWithoutStrictDriver(t *testing.T) {
	reg := strictRuntimeRegistryForTest(t, definitions.ExecutionBoth)

	_, err := NewManager(reg, exactOnlyDriverStub{inner: memory.NewMemoryDriver()}, lockobserve.NewNoopRecorder())
	if err == nil || !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation for missing strict driver capability, got %v", err)
	}
}

func TestRuntimeManagerAllowsStrictAsyncOnlyRegistryWithoutStrictDriver(t *testing.T) {
	reg := strictRuntimeRegistryForTest(t, definitions.ExecutionAsync)

	mgr, err := NewManager(reg, exactOnlyDriverStub{inner: memory.NewMemoryDriver()}, lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("expected runtime manager to allow async-only strict definitions, got %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
}

func TestExecuteExclusiveStrictPopulatesFencingToken(t *testing.T) {
	reg := strictRuntimeRegistryForTest(t, definitions.ExecutionSync)
	mgr, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
		DefinitionID: "StrictOrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "runtime-a",
		},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		if lease.FencingToken == 0 {
			t.Fatal("expected non-zero fencing token for strict runtime execution")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteExclusive returned error: %v", err)
	}
}

func TestExecuteExclusiveStrictSuiteKeepsStandardFencingTokenZero(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "runtime-a",
		},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		if lease.FencingToken != 0 {
			t.Fatalf("expected zero fencing token for standard runtime execution, got %d", lease.FencingToken)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteExclusive returned error: %v", err)
	}
}

func TestExecuteExclusiveStrictReacquireAfterReleaseIncreasesToken(t *testing.T) {
	reg := strictRuntimeRegistryForTest(t, definitions.ExecutionSync)
	mgr, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	req := definitions.SyncLockRequest{
		DefinitionID: "StrictOrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "runtime-a",
		},
	}

	var firstToken uint64
	err = mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		firstToken = lease.FencingToken
		return nil
	})
	if err != nil {
		t.Fatalf("first ExecuteExclusive returned error: %v", err)
	}

	var secondToken uint64
	err = mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		secondToken = lease.FencingToken
		return nil
	})
	if err != nil {
		t.Fatalf("second ExecuteExclusive returned error: %v", err)
	}

	if secondToken <= firstToken {
		t.Fatalf("expected fencing token to increase across reacquire, first=%d second=%d", firstToken, secondToken)
	}
}

func TestExecuteExclusiveStrictRejectsReentrantAcquire(t *testing.T) {
	reg := strictRuntimeRegistryForTest(t, definitions.ExecutionSync)
	mgr, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	req := definitions.SyncLockRequest{
		DefinitionID: "StrictOrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "runtime-a",
		},
	}

	err = mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		return mgr.ExecuteExclusive(ctx, req, func(ctx context.Context, nested definitions.LeaseContext) error {
			return nil
		})
	})

	if !errors.Is(err, lockerrors.ErrReentrantAcquire) {
		t.Fatalf("expected reentrant acquire error, got %v", err)
	}
}

func TestExecuteExclusiveDifferentOwnerHitsDriverContention(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	req := definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			ServiceName: "svc",
			InstanceID:  "one",
			HandlerName: "UpdateOrder",
			OwnerID:     "svc:one",
		},
	}

	var innerErr error
	err = mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		other := definitions.SyncLockRequest{
			DefinitionID: "OrderLock",
			KeyInput: map[string]string{
				"order_id": "123",
			},
			Ownership: definitions.OwnershipMeta{
				ServiceName: "svc",
				InstanceID:  "two",
				HandlerName: "UpdateOrder",
				OwnerID:     "svc:two",
			},
		}
		innerErr = mgr.ExecuteExclusive(ctx, other, func(ctx context.Context, nested definitions.LeaseContext) error {
			return nil
		})
		return nil
	})

	if err != nil {
		t.Fatalf("outer ExecuteExclusive returned error: %v", err)
	}
	if innerErr == nil {
		t.Fatalf("expected contention error for different owner")
	}
	if errors.Is(innerErr, lockerrors.ErrReentrantAcquire) {
		t.Fatalf("unexpected reentrant error for different owner")
	}
	if !errors.Is(innerErr, lockerrors.ErrLockBusy) {
		t.Fatalf("expected runtime contention error, got %v", innerErr)
	}
}

func TestExecuteExclusiveRejectsParentWhenChildHeldByAnotherManager(t *testing.T) {
	reg := registryWithLineageChain(t)
	driver := memory.NewMemoryDriver()

	childManager, err := NewManager(reg, driver, lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("child manager init failed: %v", err)
	}
	parentManager, err := NewManager(reg, driver, lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("parent manager init failed: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- childManager.ExecuteExclusive(context.Background(), childSyncRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	err = parentManager.ExecuteExclusive(context.Background(), parentSyncRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
		t.Fatal("parent callback should not run")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection, got %v", err)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("child ExecuteExclusive returned error: %v", err)
	}
}

func TestExecuteExclusiveRejectsChildWhenParentHeldByAnotherManager(t *testing.T) {
	reg := registryWithLineageChain(t)
	driver := memory.NewMemoryDriver()

	parentManager, err := NewManager(reg, driver, lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("parent manager init failed: %v", err)
	}
	childManager, err := NewManager(reg, driver, lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("child manager init failed: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- parentManager.ExecuteExclusive(context.Background(), parentSyncRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	err = childManager.ExecuteExclusive(context.Background(), childSyncRequest(), func(ctx context.Context, lease definitions.LeaseContext) error {
		t.Fatal("child callback should not run")
		return nil
	})
	if !errors.Is(err, lockerrors.ErrOverlapRejected) {
		t.Fatalf("expected overlap rejection, got %v", err)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("parent ExecuteExclusive returned error: %v", err)
	}
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

func strictRuntimeRegistryForTest(t *testing.T, kind definitions.ExecutionKind) *registry.Registry {
	t.Helper()

	reg := registry.New()
	def := definitions.LockDefinition{
		ID:                   "StrictOrderLock",
		Kind:                 backend.KindParent,
		Resource:             "order",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        kind,
		LeaseTTL:             30 * time.Second,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		IdempotencyRequired:  kind == definitions.ExecutionAsync || kind == definitions.ExecutionBoth,
	}
	if err := reg.Register(def); err != nil {
		t.Fatalf("register strict definition failed: %v", err)
	}
	return reg
}

func registryWithLineageChain(t *testing.T) *registry.Registry {
	t.Helper()

	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
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
		ExecutionKind: definitions.ExecutionSync,
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

func parentSyncRequest() definitions.SyncLockRequest {
	return definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "svc:parent",
		},
	}
}

func childSyncRequest() definitions.SyncLockRequest {
	return definitions.SyncLockRequest{
		DefinitionID: "ItemLock",
		KeyInput: map[string]string{
			"order_id": "123",
			"item_id":  "line-1",
		},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "svc:child",
		},
	}
}

func TestExecuteExclusiveInvalidOverridesDoesNotPoisonGuard(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	req := definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			ServiceName: "svc",
			InstanceID:  "one",
			HandlerName: "UpdateOrder",
			OwnerID:     "svc:one",
		},
		Overrides: &definitions.RuntimeOverrides{
			MaxRetries: maxRetriesPtr(1),
		},
	}

	err = mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		t.Fatalf("callback should not run when overrides are invalid")
		return nil
	})

	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation, got %v", err)
	}

	req.Overrides = nil
	called := false
	err = mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		called = true
		return nil
	})

	if err != nil {
		t.Fatalf("expected valid acquire to succeed after invalid override, got %v", err)
	}
	if !called {
		t.Fatalf("expected callback to run after guard reset")
	}
}

func TestExecuteExclusiveZeroWaitTimeoutOverride(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		WaitTimeout:   5 * time.Second,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	driver := &contextSensitiveDriver{}
	mgr, err := NewManager(reg, driver, lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
		Overrides: &definitions.RuntimeOverrides{
			WaitTimeout: durationPtr(0),
		},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		t.Fatalf("callback should not execute")
		return nil
	})

	if !errors.Is(err, lockerrors.ErrLockAcquireTimeout) {
		t.Fatalf("expected timeout error for zero override, got %v", err)
	}
}

func TestExecuteExclusiveHonorsContextDeadlineBeforeWaitTimeout(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		WaitTimeout:   5 * time.Second,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	driver := &contextSensitiveDriver{}
	mgr, err := NewManager(reg, driver, lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	err = mgr.ExecuteExclusive(ctx, definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		return nil
	})

	if !errors.Is(err, lockerrors.ErrLockAcquireTimeout) {
		t.Fatalf("expected runtime timeout error, got %v", err)
	}
	if time.Since(start) >= time.Second {
		t.Fatal("expected context deadline to stop waiting before the definition timeout")
	}
}

func TestExecuteExclusiveConcurrentSameOwnerGuard(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	driver := newBlockingDriver()
	mgr, err := NewManager(reg, driver, lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	req := definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{
			ServiceName: "svc",
			InstanceID:  "one",
			HandlerName: "UpdateOrder",
			OwnerID:     "svc:one",
		},
	}

	var wg sync.WaitGroup
	wg.Add(1)
	var outerErr error
	go func() {
		defer wg.Done()
		outerErr = mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
			return nil
		})
	}()

	driver.WaitForAcquire()

	secondErr := mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		return nil
	})

	if !errors.Is(secondErr, lockerrors.ErrReentrantAcquire) {
		t.Fatalf("expected concurrent same-owner request to be rejected, got %v", secondErr)
	}

	driver.UnblockAcquire()
	wg.Wait()

	if outerErr != nil {
		t.Fatalf("outer ExecuteExclusive failed: %v", outerErr)
	}
}

func TestExecuteExclusiveMetricsExcludePendingGuards(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	rec := &countingRecorder{}
	driver := newBlockingDriver()
	mgr, err := NewManager(reg, driver, rec)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	req := definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
			return nil
		})
	}()

	driver.WaitForAcquire()
	if len(rec.activeCounts()) != 0 {
		t.Fatalf("expected no active lock metrics while guard pending, got %v", rec.activeCounts())
	}

	driver.UnblockAcquire()
	if err := <-errCh; err != nil {
		t.Fatalf("Acquire failed: %v", err)
	}

	counts := rec.activeCounts()
	if len(counts) != 2 || counts[0] != 1 || counts[1] != 0 {
		t.Fatalf("unexpected active lock counts: %v", counts)
	}
}

func TestExecuteExclusiveCancellationPropagates(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	driver := newBlockingDriver()
	mgr, err := NewManager(reg, driver, lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	req := definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.ExecuteExclusive(ctx, req, func(ctx context.Context, lease definitions.LeaseContext) error {
			return nil
		})
	}()

	driver.WaitForAcquire()
	cancel()

	err = <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

type contextSensitiveDriver struct{}

func (contextSensitiveDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	<-ctx.Done()
	return backend.LeaseRecord{}, ctx.Err()
}

func (contextSensitiveDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	return backend.LeaseRecord{}, backend.ErrInvalidRequest
}

func (contextSensitiveDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return nil
}

func (contextSensitiveDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return backend.PresenceRecord{}, nil
}

func (contextSensitiveDriver) Ping(ctx context.Context) error {
	return nil
}

type blockingDriver struct {
	startOnce  sync.Once
	resumeOnce sync.Once
	start      chan struct{}
	resume     chan struct{}
}

func newBlockingDriver() *blockingDriver {
	return &blockingDriver{
		start:  make(chan struct{}),
		resume: make(chan struct{}),
	}
}

func (b *blockingDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	b.startOnce.Do(func() { close(b.start) })
	select {
	case <-ctx.Done():
		return backend.LeaseRecord{}, ctx.Err()
	case <-b.resume:
		now := time.Now()
		return backend.LeaseRecord{
			DefinitionID: req.DefinitionID,
			ResourceKeys: append([]string{}, req.ResourceKeys...),
			OwnerID:      req.OwnerID,
			LeaseTTL:     req.LeaseTTL,
			AcquiredAt:   now,
			ExpiresAt:    now.Add(req.LeaseTTL),
		}, nil
	}
}

func (b *blockingDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	return lease, nil
}

func (b *blockingDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return nil
}

func (b *blockingDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return backend.PresenceRecord{}, nil
}

func (b *blockingDriver) Ping(ctx context.Context) error {
	return nil
}

func (b *blockingDriver) WaitForAcquire() {
	<-b.start
}

func (b *blockingDriver) UnblockAcquire() {
	b.resumeOnce.Do(func() { close(b.resume) })
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}

func maxRetriesPtr(value int) *int {
	return &value
}

type countingRecorder struct {
	mu     sync.Mutex
	counts []int
}

func (c *countingRecorder) RecordAcquire(context.Context, string, time.Duration, bool) {}

func (c *countingRecorder) RecordContention(context.Context, string) {}

func (c *countingRecorder) RecordOverlapRejected(context.Context, string) {}

func (c *countingRecorder) RecordTimeout(context.Context, string) {}

func (c *countingRecorder) RecordActiveLocks(ctx context.Context, definitionID string, count int) {
	c.mu.Lock()
	c.counts = append(c.counts, count)
	c.mu.Unlock()
}

func (c *countingRecorder) RecordRelease(context.Context, string, time.Duration) {}

func (c *countingRecorder) RecordPresenceCheck(context.Context, string, time.Duration) {}

func (c *countingRecorder) activeCounts() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.counts...)
}

type bridgeStub struct {
	mu               sync.Mutex
	acquireStarted   int
	acquireSucceeded int
	acquireFailed    int
	contention       int
	overlapRejected  int
	released         int
	presenceChecked  int
	shutdownStarted  int
	shutdownDone     int
	lastEvent        observe.Event
	lastErr          error
}

func (b *bridgeStub) PublishRuntimeAcquireStarted(re observe.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.acquireStarted++
	b.lastEvent = re
}

func (b *bridgeStub) PublishRuntimeAcquireSucceeded(re observe.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.acquireSucceeded++
	b.lastEvent = re
}

func (b *bridgeStub) PublishRuntimeAcquireFailed(re observe.Event, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.acquireFailed++
	b.lastEvent = re
	b.lastErr = err
}

func (b *bridgeStub) PublishRuntimeContention(re observe.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.contention++
	b.lastEvent = re
}

func (b *bridgeStub) PublishRuntimeOverlapRejected(re observe.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.overlapRejected++
	b.lastEvent = re
}

func (b *bridgeStub) PublishRuntimeReleased(re observe.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.released++
	b.lastEvent = re
}

func (b *bridgeStub) PublishRuntimePresenceChecked(re observe.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.presenceChecked++
	b.lastEvent = re
}

func (b *bridgeStub) PublishRuntimeShutdownStarted() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.shutdownStarted++
}

func (b *bridgeStub) PublishRuntimeShutdownCompleted() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.shutdownDone++
}

func TestNewManagerAcceptsOptionalObservabilityOptions(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	bridge := &bridgeStub{}
	mgr, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder(), WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}

	// Without options still works.
	mgr2, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager without options returned error: %v", err)
	}
	if mgr2 == nil {
		t.Fatal("expected non-nil manager without options")
	}
}

func TestExecuteExclusiveEmitsBridgeAcquireAndRelease(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	bridge := &bridgeStub{}
	mgr, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder(), WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput:     map[string]string{"order_id": "123"},
		Ownership:    definitions.OwnershipMeta{OwnerID: "svc:one"},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteExclusive returned error: %v", err)
	}

	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if bridge.acquireStarted != 1 {
		t.Fatalf("expected 1 acquire started event, got %d", bridge.acquireStarted)
	}
	if bridge.acquireSucceeded != 1 {
		t.Fatalf("expected 1 acquire succeeded event, got %d", bridge.acquireSucceeded)
	}
	if bridge.released != 1 {
		t.Fatalf("expected 1 released event, got %d", bridge.released)
	}
	if bridge.acquireFailed != 0 {
		t.Fatalf("expected 0 acquire failed events, got %d", bridge.acquireFailed)
	}
}

func TestExecuteExclusiveEmitsBridgeContentionOnDriverContention(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	bridge := &bridgeStub{}
	driver := memory.NewMemoryDriver()
	mgr, err := NewManager(reg, driver, lockobserve.NewNoopRecorder(), WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	req := definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput:     map[string]string{"order_id": "123"},
		Ownership:    definitions.OwnershipMeta{OwnerID: "svc:one"},
	}

	err = mgr.ExecuteExclusive(context.Background(), req, func(ctx context.Context, lease definitions.LeaseContext) error {
		other := definitions.SyncLockRequest{
			DefinitionID: "OrderLock",
			KeyInput:     map[string]string{"order_id": "123"},
			Ownership:    definitions.OwnershipMeta{OwnerID: "svc:two"},
		}
		innerErr := mgr.ExecuteExclusive(ctx, other, func(ctx context.Context, nested definitions.LeaseContext) error {
			return nil
		})
		if !errors.Is(innerErr, lockerrors.ErrLockBusy) {
			t.Fatalf("expected lock busy error, got %v", innerErr)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteExclusive returned error: %v", err)
	}
	_ = driver // keep import

	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if bridge.contention < 1 {
		t.Fatalf("expected at least 1 contention event, got %d", bridge.contention)
	}
	if bridge.acquireFailed < 1 {
		t.Fatalf("expected at least 1 acquire failed event, got %d", bridge.acquireFailed)
	}
}

func TestExecuteExclusiveEmitsBridgeActiveStateChanges(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	bridge := &bridgeStub{}
	mgr, err := NewManager(reg, memory.NewMemoryDriver(), lockobserve.NewNoopRecorder(), WithBridge(bridge))
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	err = mgr.ExecuteExclusive(context.Background(), definitions.SyncLockRequest{
		DefinitionID: "OrderLock",
		KeyInput:     map[string]string{"order_id": "123"},
		Ownership:    definitions.OwnershipMeta{OwnerID: "svc:one"},
	}, func(ctx context.Context, lease definitions.LeaseContext) error {
		return nil
	})
	if err != nil {
		t.Fatalf("ExecuteExclusive returned error: %v", err)
	}

	// Verify the bridge received a succeeded event with the correct definition and resource.
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	if bridge.lastEvent.DefinitionID != "OrderLock" {
		t.Fatalf("expected last event DefinitionID=OrderLock, got %q", bridge.lastEvent.DefinitionID)
	}
	if bridge.lastEvent.ResourceID != "order:123" {
		t.Fatalf("expected last event ResourceID=order:123, got %q", bridge.lastEvent.ResourceID)
	}
}
