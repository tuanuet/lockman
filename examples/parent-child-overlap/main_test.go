package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunReportsPhase2aOverlapRejection(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	expected := []string{
		"scenario child-held-parent-rejected: overlap rejected",
		"scenario parent-held-child-rejected: overlap rejected",
		"note: phase 2a runtime now enforces parent-child overlap across managers and goroutines",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
	if strings.Contains(output, "ParentRef is metadata only") {
		t.Fatalf("example still describes pre-phase-2a behavior: %q", output)
	}
}
