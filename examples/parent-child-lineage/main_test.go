package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPrintsDependencyBoundary(t *testing.T) {
	var out bytes.Buffer

	if err := run(&out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	expected := []string{
		"parent: acquired order:123",
		"child-like nested acquire: overlap rejected",
		"note: phase 2a now enforces parent-child dependency lineage during runtime execution",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
	if strings.Contains(output, "phase1 does not enforce parent-child dependency") {
		t.Fatalf("example still describes stale pre-phase-2a semantics: %q", output)
	}
}
