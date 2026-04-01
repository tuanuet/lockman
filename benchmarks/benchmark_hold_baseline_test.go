package lockman_test

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkAdoptionHoldMemory(b *testing.B) {
	uc := benchmarkHoldUseCase("bench.hold")
	client := newBenchmarkHoldClient(b, uc)
	defer client.Shutdown(context.Background())

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		req, err := uc.With(fmt.Sprintf("hold-%d", i))
		if err != nil {
			b.Fatalf("With returned error: %v", err)
		}
		b.StartTimer()

		handle, err := client.Hold(ctx, req)
		if err != nil {
			b.Fatalf("Hold returned error: %v", err)
		}

		b.StopTimer()
		if err := client.Forfeit(ctx, uc.ForfeitWith(handle.Token())); err != nil {
			b.Fatalf("Forfeit cleanup returned error: %v", err)
		}
	}
}

func BenchmarkAdoptionForfeitMemory(b *testing.B) {
	uc := benchmarkHoldUseCase("bench.forfeit")
	client := newBenchmarkHoldClient(b, uc)
	defer client.Shutdown(context.Background())

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		req, err := uc.With(fmt.Sprintf("forfeit-%d", i))
		if err != nil {
			b.Fatalf("With returned error: %v", err)
		}
		handle, err := client.Hold(ctx, req)
		if err != nil {
			b.Fatalf("Hold setup returned error: %v", err)
		}
		forfeit := uc.ForfeitWith(handle.Token())
		b.StartTimer()

		if err := client.Forfeit(ctx, forfeit); err != nil {
			b.Fatalf("Forfeit returned error: %v", err)
		}
	}
}
