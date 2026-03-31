package lockman_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBaselineBenchmarkContract(t *testing.T) {
	root := repoRoot(t)
	src := mustReadFile(t, filepath.Join(root, "benchmark_adoption_baseline_test.go"))

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
