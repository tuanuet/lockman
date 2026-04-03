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

func TestDefineClaimOnSharesDefinitionAcrossUseCases(t *testing.T) {
	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))
	notifyUC := DefineClaimOn("notify", def)
	alertUC := DefineClaimOn("alert", def)

	if notifyUC.core.config.definitionRef != alertUC.core.config.definitionRef {
		t.Fatal("expected claim use cases sharing a definition to share the same definitionRef pointer")
	}
}

func TestDefineHoldOnRejectsStrictDefinition(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected DefineHoldOn with strict definition to panic")
		}
	}()

	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }), StrictDef())
	DefineHoldOn("hold", def)
}

func TestRunDefinitionIDRemainsPublicNameFacing(t *testing.T) {
	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))
	uc := DefineRunOn("import", def)

	if uc.DefinitionID() != "import" {
		t.Fatalf("expected DefinitionID to return use-case name 'import', got %q", uc.DefinitionID())
	}
}

func TestDefinitionIDNeverExposesInternalHashedID(t *testing.T) {
	def := DefineLock("contract", BindResourceID("order", func(v string) string { return v }))
	uc := DefineRunOn("import", def)

	id := uc.DefinitionID()
	if strings.Contains(id, "#") || strings.Contains(id, "sha") || strings.Contains(id, "hash") {
		t.Fatalf("expected DefinitionID to never expose internal hashed ID, got %q", id)
	}
}

func TestDefineRunShorthandCreatesImplicitPrivateDefinition(t *testing.T) {
	ucA := DefineRun("order.create", BindResourceID("order", func(v string) string { return v }))
	ucB := DefineRun("order.delete", BindResourceID("order", func(v string) string { return v }))

	if ucA.core.config.definitionRef == nil {
		t.Fatal("expected DefineRun to create an implicit definitionRef")
	}
	if ucB.core.config.definitionRef == nil {
		t.Fatal("expected DefineRun to create an implicit definitionRef")
	}
	if ucA.core.config.definitionRef == ucB.core.config.definitionRef {
		t.Fatal("expected shorthand use cases with different names to have separate definitionRef pointers")
	}
	if ucA.core.config.definitionRef.name != "order.create" {
		t.Fatalf("expected implicit definition name to match use case name, got %q", ucA.core.config.definitionRef.name)
	}
	if ucB.core.config.definitionRef.name != "order.delete" {
		t.Fatalf("expected implicit definition name to match use case name, got %q", ucB.core.config.definitionRef.name)
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

func TestDefineHoldShorthandCreatesImplicitPrivateDefinition(t *testing.T) {
	ucA := DefineHold("order.hold", BindResourceID("order", func(v string) string { return v }))
	ucB := DefineHold("order.manual_hold", BindResourceID("order", func(v string) string { return v }))

	if ucA.core.config.definitionRef == nil {
		t.Fatal("expected DefineHold to create an implicit definitionRef")
	}
	if ucB.core.config.definitionRef == nil {
		t.Fatal("expected DefineHold to create an implicit definitionRef")
	}
	if ucA.core.config.definitionRef == ucB.core.config.definitionRef {
		t.Fatal("expected shorthand hold use cases with different names to have separate definitionRef pointers")
	}
	if ucA.core.config.definitionRef.name != "order.hold" {
		t.Fatalf("expected implicit definition name to match use case name, got %q", ucA.core.config.definitionRef.name)
	}
}

func TestDefineClaimShorthandCreatesImplicitPrivateDefinition(t *testing.T) {
	ucA := DefineClaim("order.claim", BindResourceID("order", func(v string) string { return v }))
	ucB := DefineClaim("order.retry", BindResourceID("order", func(v string) string { return v }))

	if ucA.core.config.definitionRef == nil {
		t.Fatal("expected DefineClaim to create an implicit definitionRef")
	}
	if ucB.core.config.definitionRef == nil {
		t.Fatal("expected DefineClaim to create an implicit definitionRef")
	}
	if ucA.core.config.definitionRef == ucB.core.config.definitionRef {
		t.Fatal("expected shorthand claim use cases with different names to have separate definitionRef pointers")
	}
	if ucA.core.config.definitionRef.name != "order.claim" {
		t.Fatalf("expected implicit definition name to match use case name, got %q", ucA.core.config.definitionRef.name)
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
