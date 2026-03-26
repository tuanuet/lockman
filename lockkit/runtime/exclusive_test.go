package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
	"lockman/lockkit/observe"
	"lockman/lockkit/registry"
	"lockman/lockkit/testkit"

	lockerrors "lockman/lockkit/errors"
)

func TestExecuteExclusiveRunsCallbackWhenLockAcquired(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
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

func TestExecuteExclusiveRejectsReentrantAcquire(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
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

func TestExecuteExclusiveDifferentOwnerHitsDriverContention(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
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

func TestExecuteExclusiveHonorsContextDeadlineBeforeWaitTimeout(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:            "OrderLock",
		Kind:          definitions.KindParent,
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
	mgr, err := NewManager(reg, driver, observe.NewNoopRecorder())
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
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	driver := newBlockingDriver()
	mgr, err := NewManager(reg, driver, observe.NewNoopRecorder())
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

type contextSensitiveDriver struct{}

func (contextSensitiveDriver) Acquire(ctx context.Context, req drivers.AcquireRequest) (drivers.LeaseRecord, error) {
	<-ctx.Done()
	return drivers.LeaseRecord{}, ctx.Err()
}

func (contextSensitiveDriver) Renew(ctx context.Context, lease drivers.LeaseRecord) (drivers.LeaseRecord, error) {
	return drivers.LeaseRecord{}, drivers.ErrInvalidRequest
}

func (contextSensitiveDriver) Release(ctx context.Context, lease drivers.LeaseRecord) error {
	return nil
}

func (contextSensitiveDriver) CheckPresence(ctx context.Context, req drivers.PresenceRequest) (drivers.PresenceRecord, error) {
	return drivers.PresenceRecord{}, nil
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

func (b *blockingDriver) Acquire(ctx context.Context, req drivers.AcquireRequest) (drivers.LeaseRecord, error) {
	b.startOnce.Do(func() { close(b.start) })
	select {
	case <-ctx.Done():
		return drivers.LeaseRecord{}, ctx.Err()
	case <-b.resume:
		now := time.Now()
		return drivers.LeaseRecord{
			DefinitionID: req.DefinitionID,
			ResourceKeys: append([]string{}, req.ResourceKeys...),
			OwnerID:      req.OwnerID,
			LeaseTTL:     req.LeaseTTL,
			AcquiredAt:   now,
			ExpiresAt:    now.Add(req.LeaseTTL),
		}, nil
	}
}

func (b *blockingDriver) Renew(ctx context.Context, lease drivers.LeaseRecord) (drivers.LeaseRecord, error) {
	return lease, nil
}

func (b *blockingDriver) Release(ctx context.Context, lease drivers.LeaseRecord) error {
	return nil
}

func (b *blockingDriver) CheckPresence(ctx context.Context, req drivers.PresenceRequest) (drivers.PresenceRecord, error) {
	return drivers.PresenceRecord{}, nil
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
