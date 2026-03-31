package lockman_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAdapterBenchmarkContract(t *testing.T) {
	root := repoRoot(t)
	src := mustReadFile(t, filepath.Join(root, "benchmarks", "benchmark_adoption_adapter_test.go"))

	for _, want := range []string{
		"package lockman_test",
		"func BenchmarkAdoptionRunRedis(",
		"func BenchmarkAdoptionClaimRedis(",
		"func BenchmarkAdoptionStrictRedis(",
		"func BenchmarkAdoptionCompositeRedis(",
		"github.com/alicebob/miniredis/v2",
		"github.com/redis/go-redis/v9",
		"github.com/tuanuet/lockman/backend/redis",
		"github.com/tuanuet/lockman/idempotency/redis",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("expected adapter benchmark file to contain %q", want)
		}
	}
}

func TestBaselineBenchmarkContract(t *testing.T) {
	root := repoRoot(t)
	src := mustReadFile(t, filepath.Join(root, "benchmarks", "benchmark_adoption_baseline_test.go"))

	for _, want := range []string{
		"package lockman_test",
		"func BenchmarkAdoptionRunMemory(",
		"func BenchmarkAdoptionRunContentionMemory(",
		"func BenchmarkAdoptionClaimMemory(",
		"func BenchmarkAdoptionClaimDuplicateMemory(",
		"func BenchmarkAdoptionStrictMemory(",
		"func BenchmarkAdoptionCompositeMemory(",
		"func BenchmarkAdoptionRenewalMemory(",
		"ErrDuplicate",
		"100*time.Millisecond",
		"200 * time.Millisecond",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("expected baseline benchmark file to contain %q", want)
		}
	}
}
