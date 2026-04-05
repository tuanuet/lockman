package lockman

import (
	"context"
	"errors"
	"testing"

	"github.com/tuanuet/lockman/backend/memory"
	memstore "github.com/tuanuet/lockman/idempotency/memory"
)

func TestClaimRejectsRequestFromDifferentRegistry(t *testing.T) {
	regA := NewRegistry()
	regB := NewRegistry()
	ucA := testClaimUseCase("order.process", true)
	ucB := testClaimUseCase("order.capture", true)
	mustRegisterUseCases(t, regA, ucA)
	mustRegisterUseCases(t, regB, ucB)

	client, err := New(
		WithRegistry(regA),
		WithIdentity(Identity{OwnerID: "worker-1"}),
		WithBackend(memory.NewMemoryDriver()),
		WithIdempotency(memstore.NewStore()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	req, err := ucB.With("123", Delivery{
		MessageID:     "msg-1",
		ConsumerGroup: "orders",
		Attempt:       1,
	})
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}

	err = client.Claim(context.Background(), req, func(context.Context, Claim) error { return nil })
	if !errors.Is(err, ErrRegistryMismatch) {
		t.Fatalf("expected ErrRegistryMismatch, got %v", err)
	}
}

func TestClaimRejectsRequestBoundBeforeLaterRegistration(t *testing.T) {
	reg := NewRegistry()
	registered := testClaimUseCase("order.process", true)
	later := testClaimUseCase("order.capture", true)
	mustRegisterUseCases(t, reg, registered)

	req, err := later.With("123", Delivery{
		MessageID:     "msg-1",
		ConsumerGroup: "orders",
		Attempt:       1,
	})
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	mustRegisterUseCases(t, reg, later)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "worker-1"}),
		WithBackend(memory.NewMemoryDriver()),
		WithIdempotency(memstore.NewStore()),
	)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	err = client.Claim(context.Background(), req, func(context.Context, Claim) error { return nil })
	if !errors.Is(err, ErrUseCaseNotFound) {
		t.Fatalf("expected ErrUseCaseNotFound, got %v", err)
	}
}

func TestClaimExecutesThroughWorkerManager(t *testing.T) {
	reg := NewRegistry()
	uc := testClaimUseCase("order.process", true)
	mustRegisterUseCases(t, reg, uc)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "worker-1"}),
		WithBackend(memory.NewMemoryDriver()),
		WithIdempotency(memstore.NewStore()),
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

	var got Claim
	err = client.Claim(context.Background(), req, func(_ context.Context, claim Claim) error {
		got = claim
		return nil
	})
	if err != nil {
		t.Fatalf("Claim returned error: %v", err)
	}
	if got.UseCase != "order.process" {
		t.Fatalf("expected use case %q, got %q", "order.process", got.UseCase)
	}
	if got.ResourceKey != "order:123" {
		t.Fatalf("expected resource key %q, got %q", "order:123", got.ResourceKey)
	}
	if got.IdempotencyKey != "msg-1" {
		t.Fatalf("expected idempotency key %q, got %q", "msg-1", got.IdempotencyKey)
	}
}

func TestClaimMapsDuplicateError(t *testing.T) {
	reg := NewRegistry()
	uc := testClaimUseCase("order.process", true)
	mustRegisterUseCases(t, reg, uc)

	client, err := New(
		WithRegistry(reg),
		WithIdentity(Identity{OwnerID: "worker-1"}),
		WithBackend(memory.NewMemoryDriver()),
		WithIdempotency(memstore.NewStore()),
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

	if err := client.Claim(context.Background(), req, func(context.Context, Claim) error { return nil }); err != nil {
		t.Fatalf("first Claim returned error: %v", err)
	}

	err = client.Claim(context.Background(), req, func(context.Context, Claim) error { return nil })
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("expected ErrDuplicate, got %v", err)
	}
}
