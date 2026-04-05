package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	lockobserve "github.com/tuanuet/lockman/lockkit/observe"
	"github.com/tuanuet/lockman/lockkit/registry"
)

func TestExecuteMultipleExclusiveAcquiresAllKeys(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      5 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	drv := &mockMultipleDriver{}
	mgr := newTestMultipleManager(t, reg, drv)

	called := false
	var gotKeys []string
	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{"order:1", "order:2", "order:3"},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		called = true
		gotKeys = append([]string(nil), lc.ResourceKeys...)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("callback was not called")
	}
	if len(gotKeys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(gotKeys))
	}
}

func TestExecuteMultipleExclusiveFailsOnBusy(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      5 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	drv := &mockMultipleDriver{failOnKey: "order:2"}
	mgr := newTestMultipleManager(t, reg, drv)

	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{"order:1", "order:2", "order:3"},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		t.Fatal("callback should not be called on failure")
		return nil
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, lockerrors.ErrLockBusy) {
		t.Fatalf("expected ErrLockBusy, got: %v", err)
	}
	if drv.releaseCount != 1 {
		t.Fatalf("expected 1 release (for order:1 rollback), got %d", drv.releaseCount)
	}
}

func TestExecuteMultipleExclusiveRejectsEmptyKeys(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      5 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	mgr := newTestMultipleManager(t, reg, &mockMultipleDriver{})

	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error for empty keys")
	}
}

func TestExecuteMultipleExclusiveRejectsDuplicateKeys(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      5 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	mgr := newTestMultipleManager(t, reg, &mockMultipleDriver{})

	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{"order:1", "order:1"},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error for duplicate keys")
	}
}

func TestExecuteMultipleExclusiveRejectsShuttingDown(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      5 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	mgr := newTestMultipleManager(t, reg, &mockMultipleDriver{})
	mgr.Shutdown(context.Background())

	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{"order:1"},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error during shutdown")
	}
}

func TestExecuteMultipleExclusiveCanonicalOrder(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      5 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	drv := &mockMultipleDriver{captureOrder: true}
	mgr := newTestMultipleManager(t, reg, drv)

	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{"order:3", "order:1", "order:2"},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(drv.acquireOrder) != 3 {
		t.Fatalf("expected 3 acquires, got %d", len(drv.acquireOrder))
	}
	expected := []string{"order:1", "order:2", "order:3"}
	for i, want := range expected {
		if drv.acquireOrder[i] != want {
			t.Errorf("acquire[%d] = %q, want %q", i, drv.acquireOrder[i], want)
		}
	}
}

func TestExecuteMultipleExclusiveRejectsStrictDefinition(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:                   "order",
		Kind:                 backend.KindParent,
		Resource:             "order",
		Mode:                 definitions.ModeStrict,
		ExecutionKind:        definitions.ExecutionAsync,
		LeaseTTL:             5 * time.Second,
		KeyBuilder:           definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
		BackendFailurePolicy: definitions.BackendFailClosed,
		FencingRequired:      true,
		IdempotencyRequired:  true,
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	mgr := newTestMultipleManager(t, reg, &mockMultipleDriver{})

	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{"order:1"},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error for strict definition")
	}
}

func TestExecuteMultipleExclusiveAggregatesMinTTL(t *testing.T) {
	reg := registry.New()
	def := definitions.LockDefinition{
		ID:            "order",
		Kind:          backend.KindParent,
		Resource:      "order",
		Mode:          definitions.ModeStandard,
		ExecutionKind: definitions.ExecutionSync,
		LeaseTTL:      5 * time.Second,
		KeyBuilder:    definitions.MustTemplateKeyBuilder("{resource_key}", []string{"resource_key"}),
	}
	if err := reg.Register(def); err != nil {
		t.Fatal(err)
	}

	drv := &mockMultipleDriver{}
	mgr := newTestMultipleManager(t, reg, drv)

	req := definitions.MultipleLockRequest{
		DefinitionID: "order",
		Keys:         []string{"order:1", "order:2"},
		Ownership: definitions.OwnershipMeta{
			OwnerID: "test-owner",
		},
	}

	var gotTTL time.Duration
	err := mgr.ExecuteMultipleExclusive(context.Background(), req, func(ctx context.Context, lc definitions.LeaseContext) error {
		gotTTL = lc.LeaseTTL
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotTTL != 5*time.Second {
		t.Fatalf("expected TTL 5s, got %v", gotTTL)
	}
}

type mockMultipleDriver struct {
	failOnKey    string
	captureOrder bool
	acquireOrder []string
	releaseCount int
}

func (d *mockMultipleDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	if d.captureOrder {
		d.acquireOrder = append(d.acquireOrder, req.ResourceKeys[0])
	}
	if req.ResourceKeys[0] == d.failOnKey {
		return backend.LeaseRecord{}, backend.ErrLeaseAlreadyHeld
	}
	return backend.LeaseRecord{
		DefinitionID: req.DefinitionID,
		ResourceKeys: req.ResourceKeys,
		OwnerID:      req.OwnerID,
		AcquiredAt:   time.Now(),
		ExpiresAt:    time.Now().Add(req.LeaseTTL),
		LeaseTTL:     req.LeaseTTL,
	}, nil
}

func (d *mockMultipleDriver) Renew(ctx context.Context, rec backend.LeaseRecord) (backend.LeaseRecord, error) {
	return rec, nil
}

func (d *mockMultipleDriver) Release(ctx context.Context, rec backend.LeaseRecord) error {
	d.releaseCount++
	return nil
}

func (d *mockMultipleDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return backend.PresenceRecord{Present: false}, nil
}

func (d *mockMultipleDriver) Ping(ctx context.Context) error {
	return nil
}

func newTestMultipleManager(t *testing.T, reg *registry.Registry, drv backend.Driver) *Manager {
	t.Helper()
	mgr, err := NewManager(reg, drv, lockobserve.NewNoopRecorder())
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}
	return mgr
}
