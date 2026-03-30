package lockman_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackendRedisModuleGoTest(t *testing.T) {
	if os.Getenv("GOWORK") == "off" {
		t.Skip("workspace-specific backend redis module check is not applicable with GOWORK=off")
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	wd, err = filepath.EvalSymlinks(wd)
	if err != nil {
		t.Fatalf("eval symlinks for wd: %v", err)
	}

	redisModPath := filepath.Join(wd, "backend", "redis", "go.mod")
	envMod := exec.Command("go", "env", "GOMOD")
	envMod.Dir = filepath.Join(wd, "backend", "redis")
	envMod.Env = append(os.Environ(), "GOWORK="+filepath.Join(wd, "go.work"))
	modOut, err := envMod.CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD in ./backend/redis failed: %v\n%s", err, string(modOut))
	}
	got := strings.TrimSpace(string(modOut))
	got, err = filepath.EvalSymlinks(got)
	if err != nil {
		t.Fatalf("eval symlinks for GOMOD path: %v", err)
	}
	if want := redisModPath; got != want {
		t.Fatalf("expected ./backend/redis to be its own module (GOMOD=%q), got %q", want, got)
	}

	cmd := exec.Command("go", "test", "./...", "-count=1")
	cmd.Dir = filepath.Join(wd, "backend", "redis")
	cmd.Env = append(os.Environ(), "GOWORK="+filepath.Join(wd, "go.work"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test ./... in ./backend/redis failed: %v\n%s", err, string(out))
	}
}
