package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPrintsStrictRuntimeFlow(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	expected := []string{
		"strict runtime lock: order:123",
		"fencing token first: 1",
		"fencing token second: 2",
		"teaching point: strict runtime exposes fencing tokens but still relies on one ttl window in phase3a",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
