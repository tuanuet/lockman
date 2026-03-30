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

func TestLegacyShimFilesAreRemoved(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	for _, rel := range []string{
		"lockkit/drivers/contracts.go",
		"lockkit/idempotency/contracts.go",
		"lockkit/idempotency/memory_store.go",
	} {
		path := filepath.Join(root, rel)
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("legacy shim file still exists: %s", rel)
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}

func TestTTLExampleDoesNotImportLegacyDriversPackage(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)
	example := filepath.Join(root, "examples", "lease-ttl-expiry", "main.go")

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, example, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse lease ttl example: %v", err)
	}

	for _, imp := range parsed.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		if path == "lockman/lockkit/drivers" {
			t.Fatalf("lease ttl example still imports legacy package %q", path)
		}
	}
}
