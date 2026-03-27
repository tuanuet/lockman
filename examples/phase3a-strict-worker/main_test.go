package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunPrintsStrictWorkerFlow(t *testing.T) {
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
		"strict worker claim: order:123",
		"fencing token: 1",
		"idempotency after ack: completed",
		"teaching point: strict worker exposes fencing tokens; guarded writes still arrive in phase3b",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
