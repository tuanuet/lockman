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
		"child-like nested acquire: acquired order:123:item:1",
		"note: nested child acquire succeeded because phase1 does not enforce parent-child dependency",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
