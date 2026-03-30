package lockman_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBackendRedisModuleLivesUnderBackendNamespace(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	goModPath := filepath.Join(root, "backend", "redis", "go.mod")
	src, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("read backend redis go.mod: %v", err)
	}

	if !strings.Contains(string(src), "module lockman/backend/redis") {
		t.Fatalf("expected backend redis module path in %s", goModPath)
	}
}
