package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunPrintsGuardedWorkerFlow(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("LOCKMAN_REDIS_URL"))
	if redisURL == "" {
		t.Skip("LOCKMAN_REDIS_URL is not set")
	}
	postgresDSN := strings.TrimSpace(os.Getenv("LOCKMAN_POSTGRES_DSN"))
	if postgresDSN == "" {
		t.Skip("LOCKMAN_POSTGRES_DSN is not set")
	}

	var out bytes.Buffer
	if err := run(&out, redisURL, postgresDSN); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	expected := []string{
		"first worker claim token: 1",
		"first guarded outcome: applied",
		"second worker claim token: 2",
		"second guarded outcome: applied",
		"late stale outcome: stale_rejected",
		"idempotency after ack: completed",
		"teaching point: phase3b carries the strict fencing token into the database write path",
		"shutdown: ok",
	}

	output := out.String()
	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
