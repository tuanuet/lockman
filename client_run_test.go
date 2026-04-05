package lockman

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tuanuet/lockman/backend/memory"
)

func TestRunRejectsRequestFromDifferentRegistry(t *testing.T) {
	regA := NewRegistry()
	regB := NewRegistry()
	ucA := testRunUseCase("order.approve")
	ucB := testRunUseCase("order.capture")
	mustRegisterUseCases(t, regA, ucA)
	mustRegisterUseCases(t, regB, ucB)

	client, err := New(
		WithRegistry(regA),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(memory.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := ucB.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	err = client.Run(context.Background(), req, func(context.Context, Lease) error { return nil })
	if !errors.Is(err, ErrRegistryMismatch) {
		t.Fatalf("expected ErrRegistryMismatch, got %v", err)
	}
}

func TestRunRejectsUnregisteredUseCase(t *testing.T) {
	reg := NewRegistry()
	registered := testRunUseCase("order.approve")
	unregistered := testRunUseCase("order.capture")
	mustRegisterUseCases(t, reg, registered)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(memory.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := unregistered.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	err = client.Run(context.Background(), req, func(context.Context, Lease) error { return nil })
	if !errors.Is(err, ErrUseCaseNotFound) {
		t.Fatalf("expected ErrUseCaseNotFound, got %v", err)
	}
}

func TestRunRejectsRequestBoundBeforeLaterRegistration(t *testing.T) {
	reg := NewRegistry()
	registered := testRunUseCase("order.approve")
	later := testRunUseCase("order.capture")
	mustRegisterUseCases(t, reg, registered)

	req, err := later.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	mustRegisterUseCases(t, reg, later)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(memory.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	err = client.Run(context.Background(), req, func(context.Context, Lease) error { return nil })
	if !errors.Is(err, ErrUseCaseNotFound) {
		t.Fatalf("expected ErrUseCaseNotFound, got %v", err)
	}
}

func TestRunExecutesThroughRuntimeManager(t *testing.T) {
	reg := NewRegistry()
	uc := testRunUseCase("order.approve")
	mustRegisterUseCases(t, reg, uc)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{
			OwnerID:  "owner-1",
			Service:  "orders",
			Instance: "api-1",
		}),
		WithBackend(memory.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := uc.With("123")
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
	if got.UseCase != "order.approve" {
		t.Fatalf("expected use case %q, got %q", "order.approve", got.UseCase)
	}
	if got.ResourceKey != "order:123" {
		t.Fatalf("expected resource key %q, got %q", "order:123", got.ResourceKey)
	}
	if got.LeaseTTL <= 0 {
		t.Fatalf("expected positive lease ttl, got %v", got.LeaseTTL)
	}
}

func TestRunMapsBusyError(t *testing.T) {
	reg := NewRegistry()
	uc := testRunUseCase("order.approve")
	mustRegisterUseCases(t, reg, uc)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "owner-1"}),
		WithBackend(memory.NewMemoryDriver()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- client.Run(context.Background(), req, func(context.Context, Lease) error {
			close(entered)
			<-release
			return nil
		})
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first run to start")
	}

	err = client.Run(context.Background(), req, func(context.Context, Lease) error { return nil })
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("expected ErrBusy, got %v", err)
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("first Run returned error: %v", err)
	}
}

func TestRunUseCaseWithCachesNormalizedUseCase(t *testing.T) {
	uc := testRunUseCase("order.approve")
	req, err := uc.With("123")
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	if req.cachedNormalized.DefinitionID() == "" {
		t.Fatal("expected With to cache normalized use case definition id")
	}
}
