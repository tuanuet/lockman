package lockman_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/advanced/composite"
	"github.com/tuanuet/lockman/advanced/strict"
	backendredis "github.com/tuanuet/lockman/backend/redis"
	idempotencyredis "github.com/tuanuet/lockman/idempotency/redis"
)

func BenchmarkAdoptionRunRedis(b *testing.B) {
	s := newMiniRedisB(b)
	client := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	b.Cleanup(func() { _ = client.Close() })

	drv := backendredis.New(client, "")

	uc := benchmarkRunUseCase("bench.run-redis")
	reg := lockman.NewRegistry()
	registerBenchmarkRunUseCase(b, reg, uc)

	cl, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "bench-runner"}),
		lockman.WithBackend(drv),
	)
	if err != nil {
		b.Fatalf("New returned error: %v", err)
	}
	defer cl.Shutdown(context.Background())

	req, err := uc.With("123")
	if err != nil {
		b.Fatalf("With returned error: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cl.Run(context.Background(), req, func(context.Context, lockman.Lease) error {
			return nil
		}); err != nil {
			b.Fatalf("Run returned error: %v", err)
		}
	}
}

func BenchmarkAdoptionClaimRedis(b *testing.B) {
	s := newMiniRedisB(b)
	client := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	b.Cleanup(func() { _ = client.Close() })

	drv := backendredis.New(client, "")
	store := idempotencyredis.New(client, "")

	uc := benchmarkClaimUseCase("bench.claim-redis", true)
	reg := lockman.NewRegistry()
	registerBenchmarkClaimUseCase(b, reg, uc)

	cl, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "bench-worker"}),
		lockman.WithBackend(drv),
		lockman.WithIdempotency(store),
	)
	if err != nil {
		b.Fatalf("New returned error: %v", err)
	}
	defer cl.Shutdown(context.Background())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msgID := fmt.Sprintf("msg-claim-redis-%d", i)
		req := benchmarkClaimRequest(b, uc, msgID)
		if err := cl.Claim(context.Background(), req, func(context.Context, lockman.Claim) error {
			return nil
		}); err != nil {
			b.Fatalf("Claim returned error: %v", err)
		}
	}
}

func BenchmarkAdoptionStrictRedis(b *testing.B) {
	s := newMiniRedisB(b)
	client := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	b.Cleanup(func() { _ = client.Close() })

	drv := backendredis.New(client, "")

	uc := strict.DefineRun[string](
		"bench.strict-redis",
		lockman.BindResourceID("order", func(v string) string { return v }),
	)
	reg := lockman.NewRegistry()
	if err := reg.Register(uc); err != nil {
		b.Fatalf("Register returned error: %v", err)
	}
	cl, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "bench-runner"}),
		lockman.WithBackend(drv),
	)
	if err != nil {
		b.Fatalf("New returned error: %v", err)
	}
	defer cl.Shutdown(context.Background())

	req, err := uc.With("123")
	if err != nil {
		b.Fatalf("With returned error: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cl.Run(context.Background(), req, func(context.Context, lockman.Lease) error {
			return nil
		}); err != nil {
			b.Fatalf("Run returned error: %v", err)
		}
	}
}

func BenchmarkAdoptionCompositeRedis(b *testing.B) {
	type compositeInput struct {
		A string
		B string
	}
	input := compositeInput{A: "a-1", B: "b-1"}

	s := newMiniRedisB(b)
	client := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	b.Cleanup(func() { _ = client.Close() })

	drv := backendredis.New(client, "")

	uc := composite.DefineRun[compositeInput]("bench.composite-redis",
		composite.DefineMember("alpha", lockman.BindResourceID("alpha", func(in compositeInput) string { return in.A })),
		composite.DefineMember("beta", lockman.BindResourceID("beta", func(in compositeInput) string { return in.B })),
	)

	reg := lockman.NewRegistry()
	if err := reg.Register(uc); err != nil {
		b.Fatalf("Register returned error: %v", err)
	}
	cl, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "bench-runner"}),
		lockman.WithBackend(drv),
	)
	if err != nil {
		b.Fatalf("New returned error: %v", err)
	}
	defer cl.Shutdown(context.Background())

	req, err := uc.With(input)
	if err != nil {
		b.Fatalf("With returned error: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := cl.Run(context.Background(), req, func(context.Context, lockman.Lease) error {
			return nil
		}); err != nil {
			b.Fatalf("Run returned error: %v", err)
		}
	}
}

func newMiniRedisB(b *testing.B) *miniredis.Miniredis {
	b.Helper()
	s, err := miniredis.Run()
	if err != nil {
		b.Fatalf("miniredis run failed: %v", err)
	}
	b.Cleanup(s.Close)
	return s
}
