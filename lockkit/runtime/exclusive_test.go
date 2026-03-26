package runtime

import (
	"context"
	"errors"
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
		err := mgr.ExecuteExclusive(ctx, other, func(ctx context.Context, nested definitions.LeaseContext) error {
			return nil
		})
		if err == nil {
			t.Fatalf("expected different owner to hit contention")
		}
		if errors.Is(err, lockerrors.ErrReentrantAcquire) {
			t.Fatalf("expected contention path to run for different owner, got reentrant error")
		}
		if !errors.Is(err, drivers.ErrLeaseAlreadyHeld) {
			t.Fatalf("expected driver contention error, got %v", err)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("outer ExecuteExclusive returned error: %v", err)
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

	driver := testkit.NewMemoryDriver()
	heldLease, err := driver.Acquire(context.Background(), drivers.AcquireRequest{
		DefinitionID: "OrderLock",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "svc:other",
		LeaseTTL:     30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}
	defer driver.Release(context.Background(), heldLease)

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

	if err == nil {
		t.Fatal("expected timeout or context cancellation")
	}
	if time.Since(start) >= time.Second {
		t.Fatal("expected context deadline to beat wait timeout")
	}
}
