package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPrintsCompositeSyncFlow(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	expected := []string{
		"composite acquired: account:acct-123,ledger:ledger-456",
		"canonical order: ok",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
