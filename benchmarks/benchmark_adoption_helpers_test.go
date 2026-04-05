package lockman_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/backend/memory"
	memstore "github.com/tuanuet/lockman/idempotency/memory"
)

func registerBenchmarkRunUseCase(b *testing.B, reg *lockman.Registry, uc lockman.RunUseCase[string]) {
	b.Helper()
	if err := reg.Register(uc); err != nil {
		b.Fatalf("Register returned error: %v", err)
	}
}

func registerBenchmarkClaimUseCase(b *testing.B, reg *lockman.Registry, uc lockman.ClaimUseCase[string]) {
	b.Helper()
	if err := reg.Register(uc); err != nil {
		b.Fatalf("Register returned error: %v", err)
	}
}

func registerBenchmarkHoldUseCase(b *testing.B, reg *lockman.Registry, uc lockman.HoldUseCase[string]) {
	b.Helper()
	if err := reg.Register(uc); err != nil {
		b.Fatalf("Register returned error: %v", err)
	}
}

func benchmarkRunUseCase(name string) lockman.RunUseCase[string] {
	def := lockman.DefineLock(name, lockman.BindResourceID("order", func(v string) string { return v }))
	return lockman.DefineRunOn[string](name, def)
}

func benchmarkClaimUseCase(name string, idempotent bool) lockman.ClaimUseCase[string] {
	opts := []lockman.UseCaseOption{}
	if idempotent {
		opts = append(opts, lockman.Idempotent())
	}
	def := lockman.DefineLock(name, lockman.BindResourceID("order", func(v string) string { return v }))
	return lockman.DefineClaimOn[string](name, def, opts...)
}

func benchmarkHoldUseCase(name string) lockman.HoldUseCase[string] {
	def := lockman.DefineLock(name, lockman.BindResourceID("order", func(v string) string { return v }))
	return lockman.DefineHoldOn[string](name, def, lockman.TTL(15*time.Minute))
}

func newBenchmarkRunClient(b *testing.B, uc lockman.RunUseCase[string]) (*lockman.Client, lockman.RunRequest) {
	b.Helper()
	reg := lockman.NewRegistry()
	registerBenchmarkRunUseCase(b, reg, uc)
	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "bench-runner"}),
		lockman.WithBackend(memory.NewMemoryDriver()),
	)
	if err != nil {
		b.Fatalf("New returned error: %v", err)
	}
	req, err := uc.With("123")
	if err != nil {
		b.Fatalf("With returned error: %v", err)
	}
	return client, req
}

func newBenchmarkClaimClient(b *testing.B, uc lockman.ClaimUseCase[string]) *lockman.Client {
	b.Helper()
	reg := lockman.NewRegistry()
	registerBenchmarkClaimUseCase(b, reg, uc)
	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "bench-worker"}),
		lockman.WithBackend(memory.NewMemoryDriver()),
		lockman.WithIdempotency(memstore.NewStore()),
	)
	if err != nil {
		b.Fatalf("New returned error: %v", err)
	}
	return client
}

func newBenchmarkHoldClient(b *testing.B, uc lockman.HoldUseCase[string]) *lockman.Client {
	b.Helper()
	reg := lockman.NewRegistry()
	registerBenchmarkHoldUseCase(b, reg, uc)
	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "bench-holder"}),
		lockman.WithBackend(memory.NewMemoryDriver()),
	)
	if err != nil {
		b.Fatalf("New returned error: %v", err)
	}
	return client
}

func benchmarkClaimRequest(b *testing.B, uc lockman.ClaimUseCase[string], msgID string) lockman.ClaimRequest {
	b.Helper()
	req, err := uc.With("123", lockman.Delivery{
		MessageID:     msgID,
		ConsumerGroup: "orders",
		Attempt:       1,
	})
	if err != nil {
		b.Fatalf("With returned error: %v", err)
	}
	return req
}

func benchmarkRunContentionClientPair(b *testing.B, uc lockman.RunUseCase[string]) (*lockman.Client, *lockman.Client, lockman.RunRequest) {
	b.Helper()
	reg := lockman.NewRegistry()
	registerBenchmarkRunUseCase(b, reg, uc)
	holder, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "bench-holder"}),
		lockman.WithBackend(memory.NewMemoryDriver()),
	)
	if err != nil {
		b.Fatalf("New holder returned error: %v", err)
	}
	competitor, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "bench-competitor"}),
		lockman.WithBackend(memory.NewMemoryDriver()),
	)
	if err != nil {
		b.Fatalf("New competitor returned error: %v", err)
	}
	req, err := uc.With("123")
	if err != nil {
		b.Fatalf("With returned error: %v", err)
	}
	return holder, competitor, req
}

func benchmarkRunLogFmt(name string, members int) string {
	if members > 0 {
		return fmt.Sprintf("%s/members=%d", name, members)
	}
	return name
}

func assertErrDuplicate(b *testing.B, err error) {
	b.Helper()
	if !errors.Is(err, lockman.ErrDuplicate) {
		b.Fatalf("expected ErrDuplicate, got %v", err)
	}
}
