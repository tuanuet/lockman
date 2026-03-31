package lockman_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestREADMEPointsSharedAggregateUsersToSDKExampleAndErrorsGuide(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	readme := readRepoFile(t, filepath.Join(root, "README.md"))
	for _, want := range []string{
		"examples/sdk/shared-aggregate-split-definitions",
		"docs/errors.md",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("expected README to contain %q", want)
		}
	}

	if strings.Contains(readme, "examples/core/shared-aggregate-split-definitions") {
		t.Fatal("expected README to prefer the SDK shared-aggregate example over the core copy")
	}
}

func TestProductionGuideMentionsSDKSharedAggregateExample(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	guide := readRepoFile(t, filepath.Join(root, "docs", "production-guide.md"))
	if !strings.Contains(guide, "examples/sdk/shared-aggregate-split-definitions") {
		t.Fatal("expected production guide to mention the SDK shared-aggregate example")
	}
	if !strings.Contains(guide, "`Claim` can use idempotency wiring") {
		t.Fatal("expected production guide to describe idempotency wiring as conditional for Claim")
	}
}

func TestAsyncDocsKeepOptionalIdempotencyStoryExplicit(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(file)

	quickstart := readRepoFile(t, filepath.Join(root, "docs", "quickstart-async.md"))
	if !strings.Contains(quickstart, "lockman.Idempotent()") {
		t.Fatal("expected async quickstart to show explicit Idempotent() configuration")
	}

	reference := readRepoFile(t, filepath.Join(root, "docs", "lock-definition-reference.md"))
	if !strings.Contains(reference, "lockman.Idempotent()") {
		t.Fatal("expected lock definition reference to mention Idempotent() for claim deduplication")
	}

	runtimeVsWorkers := readRepoFile(t, filepath.Join(root, "docs", "runtime-vs-workers.md"))
	if !strings.Contains(runtimeVsWorkers, "| Do you need an idempotency store? | Usually no | Usually yes |") {
		t.Fatal("expected run-vs-claim guide to keep idempotency guidance non-mandatory for every claim")
	}
}
