package lockman_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProductionGuideForApplicationTeams(t *testing.T) {
	root := repoRoot(t)
	src := mustReadFile(t, filepath.Join(root, "docs", "production-guide.md"))

	for _, want := range []string{
		"# Production Guide",
		"## Start Here",
		"## Choose Run Or Claim",
		"## Minimum Production Wiring",
		"## Stay On The Default Path",
		"## When Strict Is Worth It",
		"## When Composite Is Worth It",
		"## TTL And Renewal Mindset",
		"## Identity And Ownership",
		"## Production Checklist",
		"## Common Mistakes",
		"## Which Example To Copy",
		"`Claim` requires idempotency wiring",
		"backend/redis",
		"idempotency/redis",
		"fail fast",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("expected production guide to contain %q", want)
		}
	}
}

func TestREADMELinksAdoptionDocs(t *testing.T) {
	root := repoRoot(t)
	src := mustReadFile(t, filepath.Join(root, "README.md"))

	for _, want := range []string{
		"docs/production-guide.md",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("expected README to contain %q", want)
		}
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(src)
}
