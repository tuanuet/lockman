//go:build lockman_examples

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunPrintsSharedAggregateRuntimeWorkerFlow(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("LOCKMAN_REDIS_URL"))
	if redisURL == "" {
		t.Skip("LOCKMAN_REDIS_URL is not set")
	}

	var out bytes.Buffer
	if err := run(&out, redisURL); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	expected := []string{
		"runtime path: acquired order:123",
		"runtime definition: OrderApprovalSync",
		"worker path: claimed order:123",
		"worker definition: OrderApprovalAsync",
		"shared aggregate key: order:123",
		"teaching point: split sync and async definitions can still share one aggregate boundary",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
