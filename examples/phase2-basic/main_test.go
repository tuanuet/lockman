//go:build lockman_examples

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunPrintsPhase2WorkerFlow(t *testing.T) {
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
		"execute: callback running for order:123",
		"presence while held: held",
		"idempotency after ack: completed",
		"presence after release: not_held",
		"duplicate outcome: ignored",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
