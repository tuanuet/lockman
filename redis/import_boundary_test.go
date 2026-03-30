package redis

import (
	"io/fs"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRedisModuleDoesNotImportLockkitPackages(t *testing.T) {
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
		t.Fatalf("parse redis dir: %v", err)
	}

	pkg := pkgs["redis"]
	if pkg == nil {
		t.Fatal("expected redis package files present")
	}

	for name, file := range pkg.Files {
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(path, "lockman/lockkit/") {
				t.Fatalf("%s imports forbidden lockkit package %q", name, path)
			}
		}
	}
}
