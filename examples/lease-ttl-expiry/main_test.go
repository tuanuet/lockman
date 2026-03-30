package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPrintsTTLFlow(t *testing.T) {
	var out bytes.Buffer

	if err := run(&out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	expected := []string{
		"owner-a: acquired order:123",
		"owner-b before ttl: lock busy",
		"owner-b after ttl: acquired order:123",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
