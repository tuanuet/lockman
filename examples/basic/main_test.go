package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPrintsPhase1Flow(t *testing.T) {
	var out bytes.Buffer

	if err := run(&out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	expected := []string{
		"execute: callback running for order:123",
		"presence while held: held",
		"presence after release: not_held",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
