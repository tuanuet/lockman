package lockman_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/backend/memory"
	"github.com/tuanuet/lockman/lockkit/definitions"
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
				lockman.WithBackend(memory.NewMemoryDriver()),
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

func BenchmarkKeyBuilderBuildSingle(b *testing.B) {
	builder := definitions.MustTemplateKeyBuilder("order:{order_id}", []string{"order_id"})
	input := map[string]string{"order_id": "12345"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := builder.Build(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkKeyBuilderBuildMulti(b *testing.B) {
	builder := definitions.MustTemplateKeyBuilder("order:{order_id}:item:{item_id}", []string{"order_id", "item_id"})
	input := map[string]string{"order_id": "12345", "item_id": "789"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := builder.Build(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}
