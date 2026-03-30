package lockman_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExamplesTreeIsPartitionedIntoCoreAndSDK(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	examplesDir := filepath.Join(root, "examples")
	entries, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatalf("read %s: %v", examplesDir, err)
	}

	allowed := map[string]bool{
		"README.md": true,
		"core":      true,
		"sdk":       true,
	}
	for _, entry := range entries {
		if !allowed[entry.Name()] {
			t.Fatalf("unexpected top-level entry under examples/: %s", entry.Name())
		}
	}

	required := []string{
		"examples/core",
		"examples/core/sync-composite-lock/main.go",
		"examples/core/parent-lock-over-composite/main.go",
		"examples/sdk",
		"examples/sdk/sync-approve-order/main.go",
		"examples/sdk/async-process-order/main.go",
		"examples/sdk/shared-aggregate-split-definitions/main.go",
		"examples/sdk/parent-lock-over-composite/main.go",
		"examples/sdk/sync-transfer-funds/main.go",
		"examples/sdk/sync-fenced-write/main.go",
	}
	for _, rel := range required {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
	}
}
