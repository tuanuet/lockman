package lockman_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tuanuet/lockman"
	"github.com/tuanuet/lockman/advanced/composite"
	"github.com/tuanuet/lockman/advanced/strict"
	"github.com/tuanuet/lockman/lockkit/testkit"
)

func BenchmarkAdoptionRunMemory(b *testing.B) {
	uc := benchmarkRunUseCase("bench.run")
	client, req := newBenchmarkRunClient(b, uc)
	defer client.Shutdown(context.Background())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := client.Run(context.Background(), req, func(context.Context, lockman.Lease) error {
			return nil
		}); err != nil {
			b.Fatalf("Run returned error: %v", err)
		}
	}
}

func BenchmarkAdoptionRunContentionMemory(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("contention-%d", i)
		uc := lockman.DefineRun[string](
			"bench.run-contention",
			lockman.BindResourceID("order", func(v string) string { return v }),
		)

		reg := lockman.NewRegistry()
		registerBenchmarkRunUseCase(b, reg, uc)

		holder, err := lockman.New(
			lockman.WithRegistry(reg),
			lockman.WithIdentity(lockman.Identity{OwnerID: "bench-holder"}),
			lockman.WithBackend(testkit.NewMemoryDriver()),
		)
		if err != nil {
			b.Fatalf("New holder returned error: %v", err)
		}

		competitor, err := lockman.New(
			lockman.WithRegistry(reg),
			lockman.WithIdentity(lockman.Identity{OwnerID: "bench-competitor"}),
			lockman.WithBackend(testkit.NewMemoryDriver()),
		)
		if err != nil {
			b.Fatalf("New competitor returned error: %v", err)
		}

		req, err := uc.With(key)
		if err != nil {
			b.Fatalf("With returned error: %v", err)
		}

		entered := make(chan struct{})
		release := make(chan struct{})

		go func() {
			_ = holder.Run(context.Background(), req, func(context.Context, lockman.Lease) error {
				close(entered)
				<-release
				return nil
			})
		}()

		<-entered
		_ = competitor.Run(context.Background(), req, func(context.Context, lockman.Lease) error {
			return nil
		})
		close(release)
		holder.Shutdown(context.Background())
		competitor.Shutdown(context.Background())
	}
}

func BenchmarkAdoptionClaimMemory(b *testing.B) {
	uc := benchmarkClaimUseCase("bench.claim", true)
	client := newBenchmarkClaimClient(b, uc)
	defer client.Shutdown(context.Background())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msgID := fmt.Sprintf("msg-claim-%d", i)
		req := benchmarkClaimRequest(b, uc, msgID)
		if err := client.Claim(context.Background(), req, func(context.Context, lockman.Claim) error {
			return nil
		}); err != nil {
			b.Fatalf("Claim returned error: %v", err)
		}
	}
}

func BenchmarkAdoptionClaimDuplicateMemory(b *testing.B) {
	uc := benchmarkClaimUseCase("bench.claim-duplicate", true)
	client := newBenchmarkClaimClient(b, uc)
	defer client.Shutdown(context.Background())

	msgID := "msg-duplicate-fixed"
	req := benchmarkClaimRequest(b, uc, msgID)

	if err := client.Claim(context.Background(), req, func(context.Context, lockman.Claim) error {
		return nil
	}); err != nil {
		b.Fatalf("first Claim returned error: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := client.Claim(context.Background(), req, func(context.Context, lockman.Claim) error {
			return nil
		})
		assertErrDuplicate(b, err)
	}
}

func BenchmarkAdoptionStrictMemory(b *testing.B) {
	uc := strict.DefineRun[string](
		"bench.strict",
		lockman.BindResourceID("order", func(v string) string { return v }),
	)
	reg := lockman.NewRegistry()
	if err := reg.Register(uc); err != nil {
		b.Fatalf("Register returned error: %v", err)
	}
	client, err := lockman.New(
		lockman.WithRegistry(reg),
		lockman.WithIdentity(lockman.Identity{OwnerID: "bench-runner"}),
		lockman.WithBackend(testkit.NewMemoryDriver()),
	)
	if err != nil {
		b.Fatalf("New returned error: %v", err)
	}
	defer client.Shutdown(context.Background())

	req, err := uc.With("123")
	if err != nil {
		b.Fatalf("With returned error: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := client.Run(context.Background(), req, func(context.Context, lockman.Lease) error {
			return nil
		}); err != nil {
			b.Fatalf("Run returned error: %v", err)
		}
	}
}

func BenchmarkAdoptionCompositeMemory(b *testing.B) {
	type compositeInput struct {
		A string
		B string
		C string
		D string
	}
	input := compositeInput{A: "a-1", B: "b-1", C: "c-1", D: "d-1"}

	memberCounts := []int{1, 2, 4}
	for _, count := range memberCounts {
		count := count
		b.Run(benchmarkRunLogFmt("members", count), func(b *testing.B) {
			var uc lockman.RunUseCase[compositeInput]
			switch count {
			case 1:
				uc = composite.DefineRun[compositeInput]("bench.composite.1",
					composite.DefineMember("alpha", lockman.BindResourceID("alpha", func(in compositeInput) string { return in.A })),
				)
			case 2:
				uc = composite.DefineRun[compositeInput]("bench.composite.2",
					composite.DefineMember("alpha", lockman.BindResourceID("alpha", func(in compositeInput) string { return in.A })),
					composite.DefineMember("beta", lockman.BindResourceID("beta", func(in compositeInput) string { return in.B })),
				)
			default:
				uc = composite.DefineRun[compositeInput]("bench.composite.4",
					composite.DefineMember("alpha", lockman.BindResourceID("alpha", func(in compositeInput) string { return in.A })),
					composite.DefineMember("beta", lockman.BindResourceID("beta", func(in compositeInput) string { return in.B })),
					composite.DefineMember("gamma", lockman.BindResourceID("gamma", func(in compositeInput) string { return in.C })),
					composite.DefineMember("delta", lockman.BindResourceID("delta", func(in compositeInput) string { return in.D })),
				)
			}

			reg := lockman.NewRegistry()
			if err := reg.Register(uc); err != nil {
				b.Fatalf("Register returned error: %v", err)
			}
			client, err := lockman.New(
				lockman.WithRegistry(reg),
				lockman.WithIdentity(lockman.Identity{OwnerID: "bench-runner"}),
				lockman.WithBackend(testkit.NewMemoryDriver()),
			)
			if err != nil {
				b.Fatalf("New returned error: %v", err)
			}
			defer client.Shutdown(context.Background())

			req, err := uc.With(input)
			if err != nil {
				b.Fatalf("With returned error: %v", err)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := client.Run(context.Background(), req, func(context.Context, lockman.Lease) error {
					return nil
				}); err != nil {
					b.Fatalf("Run returned error: %v", err)
				}
			}
		})
	}
}

func BenchmarkAdoptionRenewalMemory(b *testing.B) {
	uc := lockman.DefineClaim[string](
		"bench.renewal",
		lockman.BindResourceID("order", func(v string) string { return v }),
		lockman.TTL(100*time.Millisecond),
		lockman.Idempotent(),
	)
	client := newBenchmarkClaimClient(b, uc)
	defer client.Shutdown(context.Background())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		msgID := fmt.Sprintf("msg-renewal-%d", i)
		req := benchmarkClaimRequest(b, uc, msgID)
		if err := client.Claim(context.Background(), req, func(context.Context, lockman.Claim) error {
			time.Sleep(200 * time.Millisecond)
			return nil
		}); err != nil {
			b.Fatalf("Claim returned error: %v", err)
		}
	}
}
