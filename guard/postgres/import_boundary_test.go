package postgres

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPostgresGuardModuleDoesNotImportLockkitPackages(t *testing.T) {
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
		t.Fatalf("parse postgres guard dir: %v", err)
	}

	pkg := pkgs["postgres"]
	if pkg == nil {
		t.Fatal("expected postgres package files present")
	}

	for name, file := range pkg.Files {
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(path, "lockman/") {
				rest := strings.TrimPrefix(path, "lockman/")
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
