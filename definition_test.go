package lockman

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tuanuet/lockman/backend"
)

func TestDefineLockCreatesStableDefinitionID(t *testing.T) {
	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))

	if err := def.ForceRelease(context.Background(), nil, ""); err == nil {
		t.Fatal("expected ForceRelease to return error when not implemented")
	}

	id := def.stableID()
	if id == "" {
		t.Fatal("expected stable definition ID to be non-empty")
	}
	if !strings.HasPrefix(id, "contract") {
		t.Fatalf("expected stable ID to be based on name, got %q", id)
	}
}

func TestDefineLockRejectsEmptyName(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected DefineLock with empty name to panic")
		}
	}()

	DefineLock("", BindResourceID("order", func(v string) string { return v }))
}

func TestDefineLockRejectsMissingBinding(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected DefineLock with nil binding to panic")
		}
	}()

	DefineLock("order", Binding[string]{})
}

func TestDefineRunOnSharesDefinitionAcrossUseCases(t *testing.T) {
	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))
	importUC := DefineRunOn("import", def)
	deleteUC := DefineRunOn("delete", def)

	if importUC.DefinitionID() != "import" {
		t.Fatalf("expected importUC.DefinitionID() to be 'import', got %q", importUC.DefinitionID())
	}
	if deleteUC.DefinitionID() != "delete" {
		t.Fatalf("expected deleteUC.DefinitionID() to be 'delete', got %q", deleteUC.DefinitionID())
	}
	if importUC.core.config.definitionRef != deleteUC.core.config.definitionRef {
		t.Fatal("expected use cases sharing a definition to share the same definitionRef pointer")
	}
}

func TestDefineRunOnRequiresExplicitDefinitionSharing(t *testing.T) {
	defA := DefineLock("order.create", BindResourceID("order", func(v string) string { return v }))
	defB := DefineLock("order.delete", BindResourceID("order", func(v string) string { return v }))

	ucA := DefineRunOn("order.create", defA)
	ucB := DefineRunOn("order.delete", defB)

	if ucA.core.config.definitionRef == nil || ucB.core.config.definitionRef == nil {
		t.Fatal("expected explicit definitions to be attached")
	}
	if ucA.core.config.definitionRef == ucB.core.config.definitionRef {
		t.Fatal("expected independently defined use cases to keep separate definitions")
	}
}

func TestRegistryRejectsDuplicateDefinitionNamesWhenExplicitlyRegistered(t *testing.T) {
	defA := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))
	defB := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))

	ucA := DefineRunOn("import", defA)
	ucB := DefineRunOn("export", defB)

	reg := NewRegistry()
	if err := reg.Register(ucA, ucB); err == nil {
		t.Fatal("expected registry to reject use cases referencing definitions with the same name")
	}
}

func TestForceReleaseRequiresClient(t *testing.T) {
	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))

	err := def.ForceRelease(context.Background(), nil, "order-123")
	if err == nil {
		t.Fatal("expected ForceRelease to fail without a client")
	}
	if !errors.Is(err, ErrBackendRequired) {
		t.Fatalf("expected ErrBackendRequired, got %v", err)
	}
}

func TestForceReleaseRequiresBackendCapability(t *testing.T) {
	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))
	client := &Client{backend: &noOpDriver{}}

	err := def.ForceRelease(context.Background(), client, "order-123")
	if err == nil {
		t.Fatal("expected ForceRelease to fail when backend lacks force-release capability")
	}
	if !errors.Is(err, ErrBackendCapabilityRequired) {
		t.Fatalf("expected ErrBackendCapabilityRequired, got %v", err)
	}
}

func TestForceReleaseUsesSharedDefinitionID(t *testing.T) {
	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))
	backend := &mockForceReleaseDriver{t: t, wantDefinitionID: def.stableID(), wantResourceKey: "order-123"}
	client := &Client{backend: backend}

	err := def.ForceRelease(context.Background(), client, "order-123")
	if err != nil {
		t.Fatalf("expected ForceRelease to succeed, got %v", err)
	}
	if !backend.called {
		t.Fatal("expected ForceReleaseDefinition to be called on the backend")
	}
}

func TestForceReleaseIsIdempotentWhenBackendSupportsIt(t *testing.T) {
	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))
	backend := &mockForceReleaseDriver{t: t, wantDefinitionID: def.stableID(), wantResourceKey: "order-123", idempotent: true}
	client := &Client{backend: backend}

	err := def.ForceRelease(context.Background(), client, "order-123")
	if err != nil {
		t.Fatalf("expected first ForceRelease to succeed, got %v", err)
	}

	err = def.ForceRelease(context.Background(), client, "order-123")
	if err != nil {
		t.Fatalf("expected second ForceRelease to be idempotent, got %v", err)
	}
}

type noOpDriver struct{}

func (d *noOpDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	return backend.LeaseRecord{}, nil
}
func (d *noOpDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	return backend.LeaseRecord{}, nil
}
func (d *noOpDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return nil
}
func (d *noOpDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return backend.PresenceRecord{}, nil
}
func (d *noOpDriver) Ping(ctx context.Context) error {
	return nil
}

type mockForceReleaseDriver struct {
	t                *testing.T
	wantDefinitionID string
	wantResourceKey  string
	idempotent       bool
	called           bool
	callCount        int
}

func (d *mockForceReleaseDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	return backend.LeaseRecord{}, nil
}
func (d *mockForceReleaseDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	return backend.LeaseRecord{}, nil
}
func (d *mockForceReleaseDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return nil
}
func (d *mockForceReleaseDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return backend.PresenceRecord{}, nil
}
func (d *mockForceReleaseDriver) Ping(ctx context.Context) error {
	return nil
}
func (d *mockForceReleaseDriver) ForceReleaseDefinition(ctx context.Context, definitionID, resourceKey string) error {
	d.called = true
	d.callCount++
	if definitionID != d.wantDefinitionID {
		d.t.Fatalf("expected definitionID %q, got %q", d.wantDefinitionID, definitionID)
	}
	if resourceKey != d.wantResourceKey {
		d.t.Fatalf("expected resourceKey %q, got %q", d.wantResourceKey, resourceKey)
	}
	return nil
}

func TestFailIfHeldDefSetsDefinitionConfig(t *testing.T) {
	def := DefineLock("order", BindResourceID("order", func(v string) string { return v }), FailIfHeldDef())

	cfg := def.Config()
	if !cfg.FailIfHeld {
		t.Fatal("expected FailIfHeld to be true when using FailIfHeldDef()")
	}
}

func TestFailIfHeldDefCanBeCombinedWithStrictDef(t *testing.T) {
	def := DefineLock("order", BindResourceID("order", func(v string) string { return v }), StrictDef(), FailIfHeldDef())

	cfg := def.Config()
	if !cfg.Strict {
		t.Fatal("expected Strict to be true")
	}
	if !cfg.FailIfHeld {
		t.Fatal("expected FailIfHeld to be true when combined with StrictDef()")
	}
}

func TestDefinitionConfigDefaultsFailIfHeldToFalse(t *testing.T) {
	def := DefineLock("order", BindResourceID("order", func(v string) string { return v }))

	cfg := def.Config()
	if cfg.FailIfHeld {
		t.Fatal("expected FailIfHeld to default to false")
	}
}
