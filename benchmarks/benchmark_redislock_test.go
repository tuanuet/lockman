package lockman_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bsm/redislock"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tuanuet/lockman"
	backendredis "github.com/tuanuet/lockman/backend/redis"
)

func BenchmarkSyncLockRedislockRun(b *testing.B) {
	s := newMiniRedisB(b)
	client := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	b.Cleanup(func() { _ = client.Close() })

	locker := redislock.New(client)
	ctx := context.Background()
	ttl := 30 * time.Second
	key := "order:bench-redislock-run"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lock, err := locker.Obtain(ctx, key, ttl, nil)
		if err != nil {
			b.Fatalf("Obtain returned error: %v", err)
		}
		_ = lock.Release(ctx)
	}
}

func BenchmarkSyncLockLockmanRunRedis(b *testing.B) {
	s := newMiniRedisB(b)
	client := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	b.Cleanup(func() { _ = client.Close() })

	drv := backendredis.New(client, "")

	uc := benchmarkRunUseCase("bench.lockman-run-redis")
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

	req, err := uc.With("order-123")
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

func BenchmarkSyncLockRedislockRunContention(b *testing.B) {
	s := newMiniRedisB(b)
	client := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	b.Cleanup(func() { _ = client.Close() })

	locker := redislock.New(client)
	ctx := context.Background()
	ttl := 30 * time.Second
	key := "order:bench-redislock-contention"

	parallelismLevels := []int{1, 4, 16}
	for _, p := range parallelismLevels {
		p := p
		b.Run(benchmarkRunLogFmt("parallel", p), func(b *testing.B) {
			b.SetParallelism(p)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					for {
						lock, err := locker.Obtain(ctx, key, ttl, nil)
						if errors.Is(err, redislock.ErrNotObtained) {
							continue
						}
						if err != nil {
							b.Fatalf("Obtain returned error: %v", err)
						}
						if err := lock.Release(ctx); err != nil {
							b.Fatalf("Release returned error: %v", err)
						}
						break
					}
				}
			})
		})
	}
}

func BenchmarkSyncLockLockmanRunRedisContention(b *testing.B) {
	s := newMiniRedisB(b)
	client := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	b.Cleanup(func() { _ = client.Close() })

	drv := backendredis.New(client, "")

	uc := lockman.DefineRun[string](
		"bench.lockman-run-redis-contention",
		lockman.BindResourceID("order", func(v string) string { return v }),
		lockman.WaitTimeout(2*time.Second),
	)
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

	req, err := uc.With("order-contention")
	if err != nil {
		b.Fatalf("With returned error: %v", err)
	}

	parallelismLevels := []int{1, 4, 16}
	for _, p := range parallelismLevels {
		p := p
		b.Run(benchmarkRunLogFmt("parallel", p), func(b *testing.B) {
			b.SetParallelism(p)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					for {
						err := cl.Run(context.Background(), req, func(context.Context, lockman.Lease) error {
							return nil
						})
						if errors.Is(err, lockman.ErrBusy) {
							continue
						}
						if err != nil {
							b.Fatalf("Run returned error: %v", err)
						}
						break
					}
				}
			})
		})
	}
}
