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

var sdkWorkspaceExamples = []string{
	"examples/sdk/sync-approve-order/main.go",
	"examples/sdk/async-process-order/main.go",
	"examples/sdk/shared-aggregate-split-definitions/main.go",
	"examples/sdk/parent-lock-over-composite/main.go",
	"examples/sdk/sync-transfer-funds/main.go",
	"examples/sdk/sync-fenced-write/main.go",
}

var sdkWorkspaceExampleTests = []string{
	"examples/sdk/sync-approve-order/main_test.go",
	"examples/sdk/async-process-order/main_test.go",
	"examples/sdk/shared-aggregate-split-definitions/main_test.go",
	"examples/sdk/parent-lock-over-composite/main_test.go",
	"examples/sdk/sync-transfer-funds/main_test.go",
	"examples/sdk/sync-fenced-write/main_test.go",
}

func TestSDKWorkspaceExamplesExist(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	for _, rel := range sdkWorkspaceExamples {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("expected sdk workspace example %s: %v", rel, err)
		}
	}
}

func TestSDKWorkspaceExamplesUseWorkspaceBuildTag(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	for _, rel := range sdkWorkspaceExamples {
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

func TestSDKWorkspaceExampleTestsExist(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	for _, rel := range sdkWorkspaceExampleTests {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("expected sdk workspace example test %s: %v", rel, err)
		}
	}
}

func TestSDKWorkspaceExampleTestsUseWorkspaceBuildTag(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	for _, rel := range sdkWorkspaceExampleTests {
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

func TestSDKWorkspaceExamplesAvoidRemovedShimImports(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	for _, rel := range sdkWorkspaceExamples {
		path := filepath.Join(root, rel)
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}

		for _, imp := range parsed.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			switch importPath {
			case "github.com/tuanuet/lockman/lockkit/drivers", "github.com/tuanuet/lockman/lockkit/drivers/redis", "github.com/tuanuet/lockman/lockkit/idempotency", "github.com/tuanuet/lockman/lockkit/idempotency/redis", "github.com/tuanuet/lockman/lockkit/guard/postgres":
				t.Fatalf("%s still imports removed legacy path %q", rel, importPath)
			}
		}
	}
}
