package lockman

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tuanuet/lockman/backend"
	"github.com/tuanuet/lockman/idempotency"
	lockerrors "github.com/tuanuet/lockman/lockkit/errors"
	"github.com/tuanuet/lockman/lockkit/testkit"
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
	uc := testRunUseCase("order.strict")
	uc.core.config.strict = true
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
