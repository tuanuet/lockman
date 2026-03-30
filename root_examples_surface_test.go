package lockman_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var restoredRootExamples = []string{
	"examples/sync-order-approval/main.go",
	"examples/async-order-processor/main.go",
	"examples/composite-transfer/main.go",
	"examples/strict-fenced-write/main.go",
	"examples/phase2-basic/main.go",
	"examples/phase2-bulk-import-shard-worker/main.go",
	"examples/phase2-composite-worker/main.go",
	"examples/phase2-shared-aggregate-runtime-worker/main.go",
	"examples/phase2-shared-definition-contention/main.go",
	"examples/phase3a-strict-worker/main.go",
	"examples/phase3b-guarded-worker/main.go",
}

var restoredRootExampleTests = []string{
	"examples/sync-order-approval/main_test.go",
	"examples/async-order-processor/main_test.go",
	"examples/composite-transfer/main_test.go",
	"examples/strict-fenced-write/main_test.go",
	"examples/phase2-basic/main_test.go",
	"examples/phase2-bulk-import-shard-worker/main_test.go",
	"examples/phase2-composite-worker/main_test.go",
	"examples/phase2-shared-aggregate-runtime-worker/main_test.go",
	"examples/phase2-shared-definition-contention/main_test.go",
	"examples/phase3a-strict-worker/main_test.go",
	"examples/phase3b-guarded-worker/main_test.go",
}

func TestRestoredRootExamplesExist(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	for _, rel := range restoredRootExamples {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("expected restored root example %s: %v", rel, err)
		}
	}
}

func TestRestoredRootExamplesUseWorkspaceBuildTag(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	for _, rel := range restoredRootExamples {
		path := filepath.Join(root, rel)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !strings.HasPrefix(string(src), "//go:build lockman_examples\n") {
			t.Fatalf("%s must start with //go:build lockman_examples", rel)
		}
	}
}

func TestRestoredRootExampleTestsExist(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	for _, rel := range restoredRootExampleTests {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("expected restored root example test %s: %v", rel, err)
		}
	}
}

func TestRestoredRootExampleTestsUseWorkspaceBuildTag(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	for _, rel := range restoredRootExampleTests {
		path := filepath.Join(root, rel)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !strings.HasPrefix(string(src), "//go:build lockman_examples\n") {
			t.Fatalf("%s must start with //go:build lockman_examples", rel)
		}
	}
}

func TestRestoredRootExamplesAvoidRemovedShimImports(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	for _, rel := range restoredRootExamples {
		path := filepath.Join(root, rel)
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}

		for _, imp := range parsed.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			switch importPath {
			case "lockman/lockkit/drivers", "lockman/lockkit/drivers/redis", "lockman/lockkit/idempotency", "lockman/lockkit/idempotency/redis", "lockman/lockkit/guard/postgres":
				t.Fatalf("%s still imports removed legacy path %q", rel, importPath)
			}
		}
	}
}
