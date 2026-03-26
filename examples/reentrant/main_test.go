package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPrintsReentrantBoundary(t *testing.T) {
	var out bytes.Buffer

	if err := run(&out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	expected := []string{
		"outer: acquired order:123",
		"nested same lock: reentrant acquire",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
