package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPrintsParentChildRuntimeScenarios(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	expected := []string{
		"scenario parent-then-child: rejected",
		"scenario child-then-parent: rejected",
		"note: runtime overlap rejection is demonstrated through declared composite plans",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
