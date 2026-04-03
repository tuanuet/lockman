package lockman

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/idempotency"
	"github.com/tuanuet/lockman/inspect"
	"github.com/tuanuet/lockman/lockkit/definitions"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/testkit"
	"github.com/tuanuet/lockman/observe"
)

func TestNewFailsWithoutRegistry(t *testing.T) {
	_, err := New()
	if !errors.Is(err, ErrRegistryRequired) {
		t.Fatalf("expected ErrRegistryRequired, got %v", err)
	}
}

func TestNewFailsWhenIdentityHasEmptyOwnerID(t *testing.T) {
	reg := NewRegistry()
	mustRegisterUseCases(t, reg, testRunUseCase("order.approve"))

	_, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{}),
		WithBackend(testkit.NewMemoryDriver()),
	)
	if !errors.Is(err, ErrIdentityRequired) {
		t.Fatalf("expected ErrIdentityRequired, got %v", err)
	}
}

func TestNewFailsWithoutBackendWhenRegistryHasUseCases(t *testing.T) {
	reg := NewRegistry()
	mustRegisterUseCases(t, reg, testRunUseCase("order.approve"))

	_, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
	)
	if !errors.Is(err, ErrBackendRequired) {
		t.Fatalf("expected ErrBackendRequired, got %v", err)
	}
}

func TestNewFailsWhenClaimUseCaseNeedsIdempotencyButStoreMissing(t *testing.T) {
	reg := NewRegistry()
	mustRegisterUseCases(t, reg, testClaimUseCase("order.process", true))

	_, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "worker-1"}),
		WithBackend(testkit.NewMemoryDriver()),
	)
	if !errors.Is(err, ErrIdempotencyRequired) {
		t.Fatalf("expected ErrIdempotencyRequired, got %v", err)
	}
}

func TestNewAllowsNonIdempotentClaimUseCaseWithoutIdempotencyStore(t *testing.T) {
	reg := NewRegistry()
	mustRegisterUseCases(t, reg, testClaimUseCase("order.process", false))

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "worker-1"}),
		WithBackend(testkit.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("expected non-idempotent claim use case to start without idempotency store, got %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
}

func TestNewFailsWhenStrictUseCaseNeedsStrictBackendSupport(t *testing.T) {
	reg := NewRegistry()
	uc := DefineRun[string](
		"order.strict",
		BindResourceID("order", func(v string) string { return v }),
		Strict(),
	)
	mustRegisterUseCases(t, reg, uc)

	_, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(exactOnlyDriverStub{inner: testkit.NewMemoryDriver()}),
	)
	if !errors.Is(err, ErrBackendCapabilityRequired) {
		t.Fatalf("expected ErrBackendCapabilityRequired, got %v", err)
	}
	if !strings.Contains(err.Error(), "strict") {
		t.Fatalf("expected strict capability detail, got %v", err)
	}
}

func TestNewFailsWhenLineageUseCaseNeedsLineageBackendSupport(t *testing.T) {
	reg := NewRegistry()
	parent := testRunUseCase("order.parent")
	child := testRunUseCase("order.child")
	child.core.config.lineageParent = parent.core.name
	mustRegisterUseCases(t, reg, parent, child)

	_, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(exactOnlyDriverStub{inner: testkit.NewMemoryDriver()}),
	)
	if !errors.Is(err, ErrBackendCapabilityRequired) {
		t.Fatalf("expected ErrBackendCapabilityRequired, got %v", err)
	}
	if !strings.Contains(err.Error(), "lineage") {
		t.Fatalf("expected lineage capability detail, got %v", err)
	}
}

func TestNewFailsWhenHoldUseCaseUsesStrictMode(t *testing.T) {
	reg := NewRegistry()
	uc := DefineHold[string](
		"order.hold",
		BindResourceID("order", func(v string) string { return v }),
		Strict(),
	)
	mustRegisterUseCases(t, reg, uc)

	_, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
	)
	if err == nil {
		t.Fatal("expected strict hold use case startup failure")
	}
	if !strings.Contains(err.Error(), "hold use case") || !strings.Contains(err.Error(), "strict") {
		t.Fatalf("expected unsupported strict hold error, got %v", err)
	}
}

func TestNewFailsWhenHoldUseCaseUsesCompositeMode(t *testing.T) {
	reg := NewRegistry()
	uc := DefineHold[string](
		"order.hold",
		BindResourceID("order", func(v string) string { return v }),
		Composite(
			DefineCompositeMember(
				"order.primary",
				BindResourceID("order", func(v string) string { return v }),
			),
		),
	)
	mustRegisterUseCases(t, reg, uc)

	_, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
	)
	if err == nil {
		t.Fatal("expected composite hold use case startup failure")
	}
	if !strings.Contains(err.Error(), "hold use case") || !strings.Contains(err.Error(), "composite") {
		t.Fatalf("expected unsupported composite hold error, got %v", err)
	}
}

func TestNewCreatesOnlyNeededManagers(t *testing.T) {
	t.Run("run only", func(t *testing.T) {
		reg := NewRegistry()
		mustRegisterUseCases(t, reg, testRunUseCase("order.approve"))

		client, err := New(
			WithRegistry(reg),
			WithIdentity(Identity{OwnerID: "owner-1"}),
			WithBackend(testkit.NewMemoryDriver()),
		)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		if client.runtime == nil {
			t.Fatal("expected runtime manager")
		}
		if client.worker != nil {
			t.Fatal("did not expect worker manager")
		}
	})

	t.Run("claim only", func(t *testing.T) {
		reg := NewRegistry()
		mustRegisterUseCases(t, reg, testClaimUseCase("order.process", false))

		client, err := New(
			WithRegistry(reg),
			WithIdentity(Identity{OwnerID: "worker-1"}),
			WithBackend(testkit.NewMemoryDriver()),
			WithIdempotency(idempotency.NewMemoryStore()),
		)
		if err != nil {
			t.Fatalf("New returned error: %v", err)
		}
		if client.runtime != nil {
			t.Fatal("did not expect runtime manager")
		}
		if client.worker == nil {
			t.Fatal("expected worker manager")
		}
	})
}

func TestShutdownMarksClientUnavailable(t *testing.T) {
	reg := NewRegistry()
	uc := testRunUseCase("order.approve")
	mustRegisterUseCases(t, reg, uc)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if err := client.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	err = client.Run(context.Background(), req, func(context.Context, Lease) error { return nil })
	if !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("expected ErrShuttingDown after shutdown, got %v", err)
	}
}

func TestMapEngineErrorPreservesOverlapRejected(t *testing.T) {
	err := mapEngineError(lockerrors.ErrOverlapRejected, false)
	if !errors.Is(err, ErrOverlapRejected) {
		t.Fatalf("expected ErrOverlapRejected, got %v", err)
	}
}

func testRunUseCase(name string) RunUseCase[string] {
	return DefineRun[string](
		name,
		BindResourceID("order", func(v string) string { return v }),
	)
}

func testClaimUseCase(name string, idempotent bool) ClaimUseCase[string] {
	opts := []UseCaseOption{}
	if idempotent {
		opts = append(opts, Idempotent())
	}
	return DefineClaim[string](
		name,
		BindResourceID("order", func(v string) string { return v }),
		opts...,
	)
}

func mustRegisterUseCases(t *testing.T, reg *Registry, useCases ...registeredUseCase) {
	t.Helper()
	if err := reg.Register(useCases...); err != nil {
		t.Fatalf("Register returned error: %v", err)
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

func TestWithObserverPopulatesClientConfig(t *testing.T) {
	d := observe.NewDispatcher()
	defer func() { _ = d.Shutdown(context.Background()) }()

	cfg := &clientConfig{}
	opt := WithObserver(d)
	opt(cfg)

	if cfg.observer == nil {
		t.Fatal("expected observer to be set")
	}
}

func TestWithInspectStorePopulatesClientConfig(t *testing.T) {
	store := inspect.NewStore()

	cfg := &clientConfig{}
	opt := WithInspectStore(store)
	opt(cfg)

	if cfg.inspectStore == nil {
		t.Fatal("expected inspectStore to be set")
	}
}

func TestWithObservabilityPopulatesClientConfig(t *testing.T) {
	d := observe.NewDispatcher()
	defer func() { _ = d.Shutdown(context.Background()) }()
	store := inspect.NewStore()

	obs := Observability{
		Dispatcher: d,
		Store:      store,
	}

	cfg := &clientConfig{}
	opt := WithObservability(obs)
	opt(cfg)

	if cfg.observer == nil {
		t.Fatal("expected observer to be set by WithObservability")
	}
	if cfg.inspectStore == nil {
		t.Fatal("expected inspectStore to be set by WithObservability")
	}
}

func TestNewWithObserverCreatesClientWithoutError(t *testing.T) {
	d := observe.NewDispatcher()
	defer func() { _ = d.Shutdown(context.Background()) }()

	reg := NewRegistry()
	mustRegisterUseCases(t, reg, testRunUseCase("order.approve"))

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
		WithObserver(d),
	)
	if err != nil {
		t.Fatalf("New with observer returned error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
}

func TestNewWithInspectStoreCreatesClientWithoutError(t *testing.T) {
	store := inspect.NewStore()

	reg := NewRegistry()
	mustRegisterUseCases(t, reg, testRunUseCase("order.approve"))

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
		WithInspectStore(store),
	)
	if err != nil {
		t.Fatalf("New with inspect store returned error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
}

func TestNewWithObservabilityCreatesClientWithoutError(t *testing.T) {
	d := observe.NewDispatcher()
	defer func() { _ = d.Shutdown(context.Background()) }()
	store := inspect.NewStore()

	reg := NewRegistry()
	mustRegisterUseCases(t, reg, testRunUseCase("order.approve"))

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
		WithObservability(Observability{Dispatcher: d, Store: store}),
	)
	if err != nil {
		t.Fatalf("New with observability returned error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
}

func TestNewWithObservabilityDoesNotRequireUseCases(t *testing.T) {
	d := observe.NewDispatcher()
	defer func() { _ = d.Shutdown(context.Background()) }()
	store := inspect.NewStore()

	reg := NewRegistry()

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithObservability(Observability{Dispatcher: d, Store: store}),
	)
	if err != nil {
		t.Fatalf("New with observability (no use cases) returned error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
}

func TestClientWithInspectStoreUpdatesLocalStateWithoutDispatcher(t *testing.T) {
	store := inspect.NewStore()

	reg := NewRegistry()
	uc := testRunUseCase("order.approve")
	mustRegisterUseCases(t, reg, uc)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
		WithInspectStore(store),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	err = client.Run(context.Background(), req, func(context.Context, Lease) error { return nil })
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Verify the inspect store captured events (acquire succeeded + released).
	events := store.RecentEvents(10)
	if len(events) < 2 {
		t.Fatalf("expected inspect store to capture at least 2 events, got %d", len(events))
	}
}

func TestWithObserverPublishesAsyncExportWithoutInspectStore(t *testing.T) {
	var publishCount int
	d := observe.NewDispatcher(
		observe.WithExporter(observe.ExporterFunc(func(_ context.Context, _ observe.Event) error {
			publishCount++
			return nil
		})),
	)
	defer func() { _ = d.Shutdown(context.Background()) }()

	reg := NewRegistry()
	uc := testRunUseCase("order.approve")
	mustRegisterUseCases(t, reg, uc)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
		WithObserver(d),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	err = client.Run(context.Background(), req, func(context.Context, Lease) error { return nil })
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestWithObservabilityWiresBothOnce(t *testing.T) {
	d := observe.NewDispatcher()
	defer func() { _ = d.Shutdown(context.Background()) }()
	store := inspect.NewStore()

	reg := NewRegistry()
	uc := testRunUseCase("order.approve")
	mustRegisterUseCases(t, reg, uc)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
		WithObservability(Observability{Dispatcher: d, Store: store}),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	err = client.Run(context.Background(), req, func(context.Context, Lease) error { return nil })
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Verify the inspect store captured events (acquire succeeded + released).
	events := store.RecentEvents(10)
	if len(events) < 2 {
		t.Fatalf("expected inspect store to capture at least 2 events, got %d", len(events))
	}
}

func TestRunWithObservabilitySurfacesNormalizedEventFields(t *testing.T) {
	d := observe.NewDispatcher()
	defer func() { _ = d.Shutdown(context.Background()) }()
	store := inspect.NewStore()

	reg := NewRegistry()
	uc := testRunUseCase("order.approve")
	mustRegisterUseCases(t, reg, uc)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1", Service: "orders", Instance: "api-1"}),
		WithBackend(testkit.NewMemoryDriver()),
		WithObservability(Observability{Dispatcher: d, Store: store}),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	err = client.Run(context.Background(), req, func(context.Context, Lease) error { return nil })
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	// Verify the events captured by the store have correct normalized fields.
	events := store.RecentEvents(10)
	if len(events) < 2 {
		t.Fatalf("expected at least 2 events, got %d", len(events))
	}
	// Find the acquire_succeeded event.
	var found bool
	for _, e := range events {
		if e.Kind == observe.EventAcquireSucceeded {
			if e.OwnerID != "owner-1" {
				t.Fatalf("expected owner %q, got %q", "owner-1", e.OwnerID)
			}
			if e.ResourceID != "order:123" {
				t.Fatalf("expected resource %q, got %q", "order:123", e.ResourceID)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected acquire_succeeded event in captured events")
	}
}

func TestClaimWithObservabilitySurfacesNormalizedEventFields(t *testing.T) {
	d := observe.NewDispatcher()
	defer func() { _ = d.Shutdown(context.Background()) }()
	store := inspect.NewStore()

	reg := NewRegistry()
	uc := testClaimUseCase("order.process", true)
	mustRegisterUseCases(t, reg, uc)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "worker-1"}),
		WithBackend(testkit.NewMemoryDriver()),
		WithIdempotency(idempotency.NewMemoryStore()),
		WithObservability(Observability{Dispatcher: d, Store: store}),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := uc.With("123", Delivery{
		MessageID:     "msg-1",
		ConsumerGroup: "orders",
		Attempt:       1,
	})
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	err = client.Claim(context.Background(), req, func(context.Context, Claim) error { return nil })
	if err != nil {
		t.Fatalf("Claim returned error: %v", err)
	}

	snap := store.Snapshot()
	if len(snap.WorkerClaims) == 0 {
		t.Fatal("expected inspect store to capture worker claim")
	}
	claim := snap.WorkerClaims[0]
	if claim.OwnerID != "worker-1" {
		t.Fatalf("expected owner %q, got %q", "worker-1", claim.OwnerID)
	}
	if claim.ResourceID != "order:123" {
		t.Fatalf("expected resource %q, got %q", "order:123", claim.ResourceID)
	}
}

func TestClientShutdownPublishesFinalEventsThenDrainsDispatcher(t *testing.T) {
	d := observe.NewDispatcher()
	store := inspect.NewStore()

	reg := NewRegistry()
	uc := testRunUseCase("order.approve")
	mustRegisterUseCases(t, reg, uc)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
		WithObservability(Observability{Dispatcher: d, Store: store}),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	err = client.Run(context.Background(), req, func(context.Context, Lease) error { return nil })
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if err := client.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	snap := store.Snapshot()
	if !snap.Shutdown.Started {
		t.Fatal("expected shutdown started event in inspect store")
	}
	if !snap.Shutdown.Completed {
		t.Fatal("expected shutdown completed event in inspect store")
	}
}

func TestNewAllowsMultipleUseCasesToShareOneDefinition(t *testing.T) {
	reg := NewRegistry()
	def := DefineLock("order.lock", BindResourceID("order", func(v string) string { return v }))
	runUC := DefineRunOn("order.run", def)
	holdUC := DefineHoldOn("order.hold", def)
	mustRegisterUseCases(t, reg, runUC, holdUC)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}

	// Verify the engine registry has exactly one definition for the shared definition.
	defs := client.plan.engineRegistry.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected exactly 1 engine definition for shared definition, got %d", len(defs))
	}
	if defs[0].ID != def.stableID() {
		t.Fatalf("expected definition ID %q, got %q", def.stableID(), defs[0].ID)
	}
}

func TestSharedDefinitionReferencedByRunAndClaimNormalizesToExecutionBoth(t *testing.T) {
	reg := NewRegistry()
	def := DefineLock("order.lock", BindResourceID("order", func(v string) string { return v }))
	runUC := DefineRunOn("order.run", def)
	claimUC := DefineClaimOn("order.claim", def, Idempotent())
	mustRegisterUseCases(t, reg, runUC, claimUC)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
		WithIdempotency(idempotency.NewMemoryStore()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}

	defs := client.plan.engineRegistry.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected exactly 1 engine definition, got %d", len(defs))
	}
	if defs[0].ExecutionKind != definitions.ExecutionBoth {
		t.Fatalf("expected ExecutionBoth for run+claim shared definition, got %v", defs[0].ExecutionKind)
	}
}

func TestHoldOnStrictDefinitionFailsAtStartup(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when defining hold on strict definition")
		}
	}()

	reg := NewRegistry()
	def := DefineLock("order.lock", BindResourceID("order", func(v string) string { return v }), StrictDef())
	holdUC := DefineHoldOn("order.hold", def)
	mustRegisterUseCases(t, reg, holdUC)

	_, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
	)
	if err == nil {
		t.Fatal("expected strict hold definition startup failure")
	}
	if !strings.Contains(err.Error(), "hold") || !strings.Contains(err.Error(), "strict") {
		t.Fatalf("expected strict hold error, got %v", err)
	}
}

func TestRunUsesSharedDefinitionIdentityAtExecutionTime(t *testing.T) {
	reg := NewRegistry()
	def := DefineLock("order.lock", BindResourceID("order", func(v string) string { return v }))
	runUC := DefineRunOn("order.run", def)
	mustRegisterUseCases(t, reg, runUC)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := runUC.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	var got Lease
	err = client.Run(context.Background(), req, func(_ context.Context, lease Lease) error {
		got = lease
		return nil
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got.ResourceKey != "order:123" {
		t.Fatalf("expected resource key %q, got %q", "order:123", got.ResourceKey)
	}
}

func TestHoldUsesSharedDefinitionIdentityAtExecutionTime(t *testing.T) {
	reg := NewRegistry()
	def := DefineLock("order.lock", BindResourceID("order", func(v string) string { return v }))
	holdUC := DefineHoldOn("order.hold", def)
	mustRegisterUseCases(t, reg, holdUC)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "holder-1"}),
		WithBackend(testkit.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := holdUC.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	handle, err := client.Hold(context.Background(), req)
	if err != nil {
		t.Fatalf("Hold returned error: %v", err)
	}
	if handle.Token() == "" {
		t.Fatal("expected non-empty hold token")
	}
}

func TestClaimUsesSharedDefinitionIdentityAtExecutionTime(t *testing.T) {
	reg := NewRegistry()
	def := DefineLock("order.lock", BindResourceID("order", func(v string) string { return v }))
	claimUC := DefineClaimOn("order.claim", def, Idempotent())
	mustRegisterUseCases(t, reg, claimUC)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "worker-1"}),
		WithBackend(testkit.NewMemoryDriver()),
		WithIdempotency(idempotency.NewMemoryStore()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := claimUC.With("123", Delivery{
		MessageID:     "msg-1",
		ConsumerGroup: "orders",
		Attempt:       1,
	})
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	var got Claim
	err = client.Claim(context.Background(), req, func(_ context.Context, claim Claim) error {
		got = claim
		return nil
	})
	if err != nil {
		t.Fatalf("Claim returned error: %v", err)
	}
	if got.ResourceKey != "order:123" {
		t.Fatalf("expected resource key %q, got %q", "order:123", got.ResourceKey)
	}
}

func TestSharedDefinitionWithHoldAndRunNormalizesToExecutionSync(t *testing.T) {
	reg := NewRegistry()
	def := DefineLock("order.lock", BindResourceID("order", func(v string) string { return v }))
	runUC := DefineRunOn("order.run", def)
	holdUC := DefineHoldOn("order.hold", def)
	mustRegisterUseCases(t, reg, runUC, holdUC)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	defs := client.plan.engineRegistry.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected exactly 1 engine definition, got %d", len(defs))
	}
	if defs[0].ExecutionKind != definitions.ExecutionSync {
		t.Fatalf("expected ExecutionSync for run+hold shared definition, got %v", defs[0].ExecutionKind)
	}
}

func TestSharedDefinitionWithHoldAndClaimNormalizesToExecutionBoth(t *testing.T) {
	reg := NewRegistry()
	def := DefineLock("order.lock", BindResourceID("order", func(v string) string { return v }))
	claimUC := DefineClaimOn("order.claim", def, Idempotent())
	holdUC := DefineHoldOn("order.hold", def)
	mustRegisterUseCases(t, reg, claimUC, holdUC)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "worker-1"}),
		WithBackend(testkit.NewMemoryDriver()),
		WithIdempotency(idempotency.NewMemoryStore()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	defs := client.plan.engineRegistry.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected exactly 1 engine definition, got %d", len(defs))
	}
	if defs[0].ExecutionKind != definitions.ExecutionBoth {
		t.Fatalf("expected ExecutionBoth for claim+hold shared definition, got %v", defs[0].ExecutionKind)
	}
}

func TestRunAndRunSharingOneDefinitionProducesSingleEngineDefinition(t *testing.T) {
	reg := NewRegistry()
	def := DefineLock("order.lock", BindResourceID("order", func(v string) string { return v }))
	importUC := DefineRunOn("order.import", def)
	deleteUC := DefineRunOn("order.delete", def)
	mustRegisterUseCases(t, reg, importUC, deleteUC)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(testkit.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	defs := client.plan.engineRegistry.Definitions()
	if len(defs) != 1 {
		t.Fatalf("expected exactly 1 engine definition for run+run shared definition, got %d", len(defs))
	}
	if defs[0].ID != def.stableID() {
		t.Fatalf("expected definition ID %q, got %q", def.stableID(), defs[0].ID)
	}
	if defs[0].ExecutionKind != definitions.ExecutionSync {
		t.Fatalf("expected ExecutionSync for run+run shared definition, got %v", defs[0].ExecutionKind)
	}
}
