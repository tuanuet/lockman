package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/observe"
	"github.com/tuanuet/lockman/lockkit/registry"
	"github.com/tuanuet/lockman/lockkit/testkit"
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
	_, err = driver.Acquire(context.Background(), backend.AcquireRequest{
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

func TestCheckPresenceRecordsMetricsWithResolvedDefinitionID(t *testing.T) {
	def := definitions.LockDefinition{
		ID:               "CanonicalOrderLock",
		Kind:             definitions.KindParent,
		Resource:         "order",
		Mode:             definitions.ModeStandard,
		ExecutionKind:    definitions.ExecutionSync,
		LeaseTTL:         30 * time.Second,
		CheckOnlyAllowed: true,
		KeyBuilder:       definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"}),
	}

	driver := testkit.NewMemoryDriver()
	if _, err := driver.Acquire(context.Background(), backend.AcquireRequest{
		DefinitionID: def.ID,
		ResourceKeys: []string{"order:123"},
		OwnerID:      "svc:one",
		LeaseTTL:     def.LeaseTTL,
	}); err != nil {
		t.Fatalf("Acquire returned error: %v", err)
	}

	rec := &presenceMetricRecorder{}
	mgr, err := NewManager(aliasRegistry{definition: def}, driver, rec)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	_, err = mgr.CheckPresence(context.Background(), definitions.PresenceCheckRequest{
		DefinitionID: "AliasOrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}

	gotIDs := rec.presenceDefinitionIDs()
	if len(gotIDs) != 1 {
		t.Fatalf("expected one presence metric event, got %v", gotIDs)
	}
	if gotIDs[0] != def.ID {
		t.Fatalf("expected canonical definition id %q, got %q", def.ID, gotIDs[0])
	}
}

func TestCheckPresenceSkipsMetricsWhenDefinitionLookupFails(t *testing.T) {
	rec := &presenceMetricRecorder{}
	mgr, err := NewManager(registry.New(), testkit.NewMemoryDriver(), rec)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	_, err = mgr.CheckPresence(context.Background(), definitions.PresenceCheckRequest{
		DefinitionID: "MissingLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:one"},
	})
	if !errors.Is(err, lockerrors.ErrPolicyViolation) {
		t.Fatalf("expected policy violation for missing definition, got %v", err)
	}
	if gotIDs := rec.presenceDefinitionIDs(); len(gotIDs) != 0 {
		t.Fatalf("expected no presence metric events for unresolved definition, got %v", gotIDs)
	}
}

func TestCheckPresenceRemainsExactKeyOnlyWithActiveChild(t *testing.T) {
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
		t.Fatalf("register parent failed: %v", err)
	}
	if err := reg.Register(definitions.LockDefinition{
		ID:            "ItemLock",
		Kind:          definitions.KindChild,
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

	mgr, err := NewManager(reg, driver, observe.NewNoopRecorder())
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}

	childReq := backend.LineageAcquireRequest{
		DefinitionID: "ItemLock",
		ResourceKey:  "order:123:item:line-1",
		OwnerID:      "svc:child",
		LeaseTTL:     30 * time.Second,
		Lineage: backend.LineageLeaseMeta{
			LeaseID: "lease-child",
			Kind:    backend.KindChild,
			AncestorKeys: []backend.AncestorKey{
				{DefinitionID: "OrderLock", ResourceKey: "order:123"},
			},
		},
	}
	childLease, err := driver.AcquireWithLineage(context.Background(), childReq)
	if err != nil {
		t.Fatalf("child acquire failed: %v", err)
	}
	defer func() {
		_ = driver.ReleaseWithLineage(context.Background(), childLease, childReq.Lineage)
	}()

	status, err := mgr.CheckPresence(context.Background(), definitions.PresenceCheckRequest{
		DefinitionID: "OrderLock",
		KeyInput: map[string]string{
			"order_id": "123",
		},
		Ownership: definitions.OwnershipMeta{OwnerID: "svc:parent"},
	})
	if err != nil {
		t.Fatalf("CheckPresence returned error: %v", err)
	}
	if status.State != definitions.PresenceNotHeld {
		t.Fatalf("expected exact-key-only presence result, got %v", status.State)
	}
}

type pingFailDriver struct {
	inner backend.Driver
	err   error
}

func (d pingFailDriver) Acquire(ctx context.Context, req backend.AcquireRequest) (backend.LeaseRecord, error) {
	return d.inner.Acquire(ctx, req)
}

func (d pingFailDriver) Renew(ctx context.Context, lease backend.LeaseRecord) (backend.LeaseRecord, error) {
	return d.inner.Renew(ctx, lease)
}

func (d pingFailDriver) Release(ctx context.Context, lease backend.LeaseRecord) error {
	return d.inner.Release(ctx, lease)
}

func (d pingFailDriver) CheckPresence(ctx context.Context, req backend.PresenceRequest) (backend.PresenceRecord, error) {
	return d.inner.CheckPresence(ctx, req)
}

func (d pingFailDriver) Ping(ctx context.Context) error {
	return d.err
}

type aliasRegistry struct {
	definition definitions.LockDefinition
}

func (a aliasRegistry) MustGet(id string) definitions.LockDefinition {
	return a.definition
}

func (a aliasRegistry) MustGetComposite(id string) definitions.CompositeDefinition {
	panic("unexpected MustGetComposite call in presence tests")
}

func (a aliasRegistry) Validate() error {
	return nil
}

func (a aliasRegistry) Definitions() []definitions.LockDefinition {
	return []definitions.LockDefinition{a.definition}
}

type presenceMetricRecorder struct {
	mu  sync.Mutex
	ids []string
}

func (p *presenceMetricRecorder) RecordAcquire(context.Context, string, time.Duration, bool) {}

func (p *presenceMetricRecorder) RecordContention(context.Context, string) {}

func (p *presenceMetricRecorder) RecordOverlapRejected(context.Context, string) {}

func (p *presenceMetricRecorder) RecordTimeout(context.Context, string) {}

func (p *presenceMetricRecorder) RecordActiveLocks(context.Context, string, int) {}

func (p *presenceMetricRecorder) RecordRelease(context.Context, string, time.Duration) {}

func (p *presenceMetricRecorder) RecordPresenceCheck(ctx context.Context, definitionID string, duration time.Duration) {
	p.mu.Lock()
	p.ids = append(p.ids, definitionID)
	p.mu.Unlock()
}

func (p *presenceMetricRecorder) presenceDefinitionIDs() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.ids...)
}
