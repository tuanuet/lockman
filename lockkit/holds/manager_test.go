package holds

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/registry"
	"github.com/tuanuet/lockman/lockkit/testkit"
)

func TestHoldsManagerAcquireReturnsLease(t *testing.T) {
	reg := newTestRegistry(t)
	driver := testkit.NewMemoryDriver()

	mgr, err := NewManager(reg, driver)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	req := definitions.DetachedAcquireRequest{
		DefinitionID: "OrderHold",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "svc:hold:one",
	}
	lease, err := mgr.Acquire(context.Background(), req)
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	if lease.DefinitionID != req.DefinitionID {
		t.Fatalf("unexpected definition id: %q", lease.DefinitionID)
	}
	if len(lease.ResourceKeys) != 1 || lease.ResourceKeys[0] != req.ResourceKeys[0] {
		t.Fatalf("unexpected resource keys: %#v", lease.ResourceKeys)
	}
	if lease.OwnerID != req.OwnerID {
		t.Fatalf("unexpected owner id: %q", lease.OwnerID)
	}

	def := reg.MustGet(req.DefinitionID)
	if lease.LeaseTTL != def.LeaseTTL {
		t.Fatalf("unexpected lease ttl: got %v want %v", lease.LeaseTTL, def.LeaseTTL)
	}
}

func TestHoldsManagerReleaseCallsBackend(t *testing.T) {
	reg := newTestRegistry(t)
	driver := testkit.NewMemoryDriver()

	mgr, err := NewManager(reg, driver)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	req := definitions.DetachedAcquireRequest{
		DefinitionID: "OrderHold",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "svc:hold:one",
	}
	lease, err := mgr.Acquire(context.Background(), req)
	if err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	if err := mgr.Release(context.Background(), definitions.DetachedReleaseRequest{
		DefinitionID: req.DefinitionID,
		ResourceKeys: append([]string(nil), req.ResourceKeys...),
		OwnerID:      req.OwnerID,
	}); err != nil {
		t.Fatalf("Release returned error: %v", err)
	}

	presence, err := driver.CheckPresence(context.Background(), backend.PresenceRequest{
		DefinitionID: req.DefinitionID,
		ResourceKeys: append([]string(nil), req.ResourceKeys...),
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}
	if presence.Present {
		t.Fatalf("expected released lease to be absent, lease=%#v", lease)
	}
}

func TestHoldsManagerAcquireAfterShutdownRejected(t *testing.T) {
	reg := newTestRegistry(t)
	mgr, err := NewManager(reg, testkit.NewMemoryDriver())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	mgr.Shutdown()

	_, err = mgr.Acquire(context.Background(), definitions.DetachedAcquireRequest{
		DefinitionID: "OrderHold",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "svc:hold:one",
	})
	if !isErrPolicyViolation(err) {
		t.Fatalf("expected policy violation, got %v", err)
	}
}

func TestHoldsManagerAcquireUnknownDefinitionRejected(t *testing.T) {
	reg := newTestRegistry(t)
	mgr, err := NewManager(reg, testkit.NewMemoryDriver())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	_, err = mgr.Acquire(context.Background(), definitions.DetachedAcquireRequest{
		DefinitionID: "MissingHold",
		ResourceKeys: []string{"order:123"},
		OwnerID:      "svc:hold:one",
	})
	if !isErrPolicyViolation(err) {
		t.Fatalf("expected policy violation, got %v", err)
	}
}

func TestHoldsManagerNilDriverRejected(t *testing.T) {
	reg := newTestRegistry(t)

	_, err := NewManager(reg, nil)
	if !isErrPolicyViolation(err) {
		t.Fatalf("expected policy violation, got %v", err)
	}
}

func newTestRegistry(t *testing.T) *registry.Registry {
	t.Helper()

	reg := registry.New()
	if err := reg.Register(testHoldDefinition()); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	return reg
}

func testHoldDefinition() definitions.LockDefinition {
	return definitions.LockDefinition{
		ID:            "OrderHold",
		Kind:          definitions.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      30 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}
}

func isErrPolicyViolation(err error) bool {
	return errors.Is(err, lockerrors.ErrPolicyViolation)
}
