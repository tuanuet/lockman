package lockman_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCIWorkflowCoversBaselineMatrix(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	path := filepath.Join(root, ".github", "workflows", "ci.yml")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ci workflow: %v", err)
	}

	contents := string(src)
	for _, want := range []string{
		"go test ./...",
		"GOWORK=off go test ./...",
		"cd backend/redis && go test ./...",
		"cd idempotency/redis && go test ./...",
		"cd guard/postgres && go test ./...",
		"go test -tags lockman_examples ./examples/",
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("expected ci workflow to contain %q", want)
		}
	}
}
