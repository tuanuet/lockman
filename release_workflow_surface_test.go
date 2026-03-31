package lockman_test

import (
	"os"
	"os/exec"
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

func TestReleaseWorkflowExternalConsumerScriptIsValidBash(t *testing.T) {
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

	script := extractWorkflowRunBlock(t, string(src), "Verify released module installs outside the repo")
	tmpdir := t.TempDir()
	scriptPath := filepath.Join(tmpdir, "external-consumer.sh")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("write temp script: %v", err)
	}

	cmd := exec.Command("bash", "-n", scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected release workflow script to parse as bash, got %v: %s", err, output)
	}
}

func extractWorkflowRunBlock(t *testing.T, contents string, stepName string) string {
	t.Helper()

	lines := strings.Split(contents, "\n")
	stepMarker := "      - name: " + stepName
	runMarker := "        run: |"
	const runIndent = "          "

	inStep := false
	inRun := false
	var script []string

	for _, line := range lines {
		switch {
		case !inStep && line == stepMarker:
			inStep = true
		case inStep && !inRun && line == runMarker:
			inRun = true
		case inRun && strings.HasPrefix(line, runIndent):
			script = append(script, strings.TrimPrefix(line, runIndent))
		case inRun:
			return strings.Join(script, "\n") + "\n"
		case inStep && strings.HasPrefix(line, "      - name: ") && line != stepMarker:
			t.Fatalf("step %q has no run block", stepName)
		}
	}

	if inRun {
		return strings.Join(script, "\n") + "\n"
	}
	t.Fatalf("run block for step %q not found", stepName)
	return ""
}
