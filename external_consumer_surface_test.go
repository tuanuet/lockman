package lockman_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCIWorkflowCoversExternalConsumerInstall(t *testing.T) {
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
		"external-consumer",
		"go mod init lockmanconsumer",
		"github.com/tuanuet/lockman@v1.0.0",
		"github.com/tuanuet/lockman/backend/redis@v1.0.0",
		"github.com/tuanuet/lockman/idempotency/redis@v1.0.0",
		"github.com/tuanuet/lockman/guard/postgres@v1.0.0",
		"go test ./...",
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("expected ci workflow to contain %q", want)
		}
	}
}

func TestExternalConsumerSmokeFixtureImportsReleasedModules(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	path := filepath.Join(root, "testdata", "externalconsumer", "smoke_test.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read external consumer smoke fixture: %v", err)
	}

	contents := string(src)
	for _, want := range []string{
		"package externalconsumer",
		"\"github.com/tuanuet/lockman\"",
		"backendredis \"github.com/tuanuet/lockman/backend/redis\"",
		"idempotencyredis \"github.com/tuanuet/lockman/idempotency/redis\"",
		"guardpostgres \"github.com/tuanuet/lockman/guard/postgres\"",
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("expected external consumer smoke fixture to contain %q", want)
		}
	}
}
