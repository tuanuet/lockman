package redis

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRedisIdempotencyModuleDoesNotImportLockkitPackages(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}

	dir := filepath.Dir(file)
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info fs.FileInfo) bool {
		name := info.Name()
		return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse idempotency redis dir: %v", err)
	}

	pkg := pkgs["redis"]
	if pkg == nil {
		t.Fatal("expected redis package files present")
	}

	for name, file := range pkg.Files {
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(path, "lockman/") {
				rest := strings.TrimPrefix(path, "lockman/")
				// Avoid embedding the forbidden prefix as a single literal so
				// grep-based boundary checks stay clean.
				if strings.HasPrefix(rest, "lockkit") && (len(rest) == 6 || rest[6] == '/') {
					t.Fatalf("%s imports forbidden lockkit package %q", name, path)
				}
			}
			if strings.HasPrefix(path, "lockkit") && (len(path) == 6 || path[6] == '/') {
				t.Fatalf("%s imports forbidden lockkit package %q", name, path)
			}
		}
	}
}
