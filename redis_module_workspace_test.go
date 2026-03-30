package lockman_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedisModuleGoTest(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	redisModPath := filepath.Join(wd, "redis", "go.mod")
	envMod := exec.Command("go", "env", "GOMOD")
	envMod.Dir = filepath.Join(wd, "redis")
	modOut, err := envMod.CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD in ./redis failed: %v\n%s", err, string(modOut))
	}
	if got, want := strings.TrimSpace(string(modOut)), redisModPath; got != want {
		t.Fatalf("expected ./redis to be its own module (GOMOD=%q), got %q", want, got)
	}

	cmd := exec.Command("go", "test", "./...", "-count=1")
	cmd.Dir = filepath.Join(wd, "redis")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test ./... in ./redis failed: %v\n%s", err, string(out))
	}
}
