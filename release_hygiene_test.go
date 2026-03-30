package lockman_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseHygieneMatchesV1(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	readme := readRepoFile(t, filepath.Join(root, "README.md"))
	for _, unwanted := range []string{
		"still pre-release",
		"before release",
	} {
		if strings.Contains(readme, unwanted) {
			t.Fatalf("expected README to drop stale release wording %q", unwanted)
		}
	}

	doc := readRepoFile(t, filepath.Join(root, "doc.go"))
	for _, unwanted := range []string{
		"under construction",
		"placeholder root",
		"later tasks",
	} {
		if strings.Contains(doc, unwanted) {
			t.Fatalf("expected doc.go to drop stale package wording %q", unwanted)
		}
	}

	for _, required := range []string{"LICENSE", "CHANGELOG.md"} {
		if _, err := os.Stat(filepath.Join(root, required)); err != nil {
			t.Fatalf("expected repository file %s: %v", required, err)
		}
	}
}

func readRepoFile(t *testing.T, path string) string {
	t.Helper()

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(src)
}
