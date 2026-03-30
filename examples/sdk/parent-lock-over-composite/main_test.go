//go:build lockman_examples

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPrintsParentOverCompositeFlow(t *testing.T) {
	var out bytes.Buffer
	if err := run(&out); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	expected := []string{
		"aggregate lock: shipment:sh-123",
		"sub-resources involved: package-1,package-2",
		"teaching point: parent lock is enough, composite is overkill",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
