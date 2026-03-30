package lockman_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseWorkflowCoversTaggedReleases(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	path := filepath.Join(root, ".github", "workflows", "release.yml")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}

	contents := string(src)
	for _, want := range []string{
		"name: Release",
		"tags:",
		"- 'v*'",
		"- 'backend/redis/v*'",
		"- 'idempotency/redis/v*'",
		"- 'guard/postgres/v*'",
		"contents: write",
		"github.ref_type == 'tag'",
		"GITHUB_REF_NAME",
		"go test ./...",
		"softprops/action-gh-release@v2",
		"generate_release_notes: true",
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("expected release workflow to contain %q", want)
		}
	}
}

func TestReleaseWorkflowDerivesModuleSpecificConsumerCheck(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	path := filepath.Join(root, ".github", "workflows", "release.yml")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}

	contents := string(src)
	for _, want := range []string{
		`case "$GITHUB_REF_NAME" in`,
		`v*)`,
		`backend/redis/v*)`,
		`idempotency/redis/v*)`,
		`guard/postgres/v*)`,
		`go mod init lockmanreleaseconsumer`,
		`go get "$module_path@$module_version"`,
		`go test ./...`,
	} {
		if !strings.Contains(contents, want) {
			t.Fatalf("expected release workflow to contain %q", want)
		}
	}
}
