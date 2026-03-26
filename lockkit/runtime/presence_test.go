package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"lockman/lockkit/definitions"
	"lockman/lockkit/drivers"
	lockerrors "lockman/lockkit/errors"
	"lockman/lockkit/observe"
	"lockman/lockkit/registry"
	"lockman/lockkit/testkit"
)

func TestCheckPresenceReturnsPresenceHeld(t *testing.T) {
	driver := testkit.NewMemoryDriver()
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:               "OrderLock",
		Kind:             definitions.KindParent,
		Resource:         "order",
		Mode:             definitions.ModeStandard,
		ExecutionKind:    definitions.ExecutionSync,
		LeaseTTL:         30 * time.Second,
		CheckOnlyAllowed: true,
		KeyBuilder:       definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	_, err = driver.Acquire(context.Background(), drivers.AcquireRequest{
		DefinitionID: "OrderLock",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "svc:one",
		LeaseTTL:     30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	status, err := mgr.CheckPresence(context.Background(), definitions.PresenceCheckRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}
	if status.State != definitions.PresenceHeld {
		t.Fatalf("expected PresenceHeld, got %v", status.State)
	}
	if status.OwnerID != "svc:one" {
		t.Fatalf("expected owner svc:one, got %q", status.OwnerID)
	}
}

func TestCheckPresenceRejectsDefinitionWithoutCheckOnlyAllowed(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:               "OrderLock",
		Kind:             definitions.KindParent,
		Resource:         "order",
		Mode:             definitions.ModeStandard,
		ExecutionKind:    definitions.ExecutionSync,
		LeaseTTL:         30 * time.Second,
		CheckOnlyAllowed: false,
		KeyBuilder:       definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	mgr, err := NewManager(reg, testkit.NewMemoryDriver(), observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	_, err = mgr.CheckPresence(context.Background(), definitions.PresenceCheckRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	})
	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation for check-only disabled, got %v", err)
	}
}

func TestCheckPresenceReturnsPresenceUnknownWhenDriverHealthUnavailable(t *testing.T) {
	reg := registry.New()
	if err := reg.Register(definitions.LockDefinition{
		ID:               "OrderLock",
		Kind:             definitions.KindParent,
		Resource:         "order",
		Mode:             definitions.ModeStandard,
		ExecutionKind:    definitions.ExecutionSync,
		LeaseTTL:         30 * time.Second,
		CheckOnlyAllowed: true,
		KeyBuilder:       definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	sentinelErr := errors.New("driver unavailable")
	mgr, err := NewManager(reg, pingFailDriver{
		inner: testkit.NewMemoryDriver(),
		err:   sentinelErr,
	}, observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	status, err := mgr.CheckPresence(context.Background(), definitions.PresenceCheckRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	})
	if !errors.Is(err, sentinelErr) {
		t.Fatalf("expected wrapped ping error, got %v", err)
	}
	if status.State != definitions.PresenceUnknown {
		t.Fatalf("expected PresenceUnknown when health check fails, got %v", status.State)
	}
}

type pingFailDriver struct {
	inner drivers.Driver
	err   error
}

func (d pingFailDriver) Acquire(ctx context.Context, req drivers.AcquireRequest) (drivers.LeaseRecord, error) {
	return d.inner.Acquire(ctx, req)
}

func (d pingFailDriver) Renew(ctx context.Context, lease drivers.LeaseRecord) (drivers.LeaseRecord, error) {
	return d.inner.Renew(ctx, lease)
}

func (d pingFailDriver) Release(ctx context.Context, lease drivers.LeaseRecord) error {
	return d.inner.Release(ctx, lease)
}

func (d pingFailDriver) CheckPresence(ctx context.Context, req drivers.PresenceRequest) (drivers.PresenceRecord, error) {
	return d.inner.CheckPresence(ctx, req)
}

func (d pingFailDriver) Ping(ctx context.Context) error {
	return d.err
}
