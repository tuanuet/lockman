package lockman_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/lockkit/testkit"
)

func BenchmarkActiveCountParallel(b *testing.B) {
	for _, concurrency := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("goroutines=%d", concurrency), func(b *testing.B) {
			uc := benchmarkRunUseCase("bench.active-count")
			reg := lockman.NewRegistry()
			registerBenchmarkRunUseCase(b, reg, uc)

			client, err := lockman.New(
				lockman.WithRegistry(reg),
				lockman.WithIdentity(lockman.Identity{OwnerID: "bench-runner"}),
				lockman.WithBackend(testkit.NewMemoryDriver()),
			)
			if err != nil {
				b.Fatalf("New: %v", err)
			}
			defer client.Shutdown(context.Background())

			b.ResetTimer()
			var counter int
			var mu sync.Mutex
			b.SetParallelism(concurrency)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					mu.Lock()
					key := fmt.Sprintf("resource-%d", counter)
					counter++
					mu.Unlock()
					req, err := uc.With(key)
					if err != nil {
						b.Fatalf("With: %v", err)
					}
					if err := client.Run(context.Background(), req, func(context.Context, lockman.Lease) error {
						return nil
					}); err != nil {
						b.Fatalf("Run: %v", err)
					}
				}
			})
		})
	}
}
