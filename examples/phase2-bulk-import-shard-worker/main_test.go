package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunPrintsBulkImportShardWorkerFlow(t *testing.T) {
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
		"shard lock: import-shard:07",
		"package: workers",
		"teaching point: shard ownership is the default boundary for bulk import",
		"contrast: smaller batch locks only work when batches are independently safe and replayable",
		"shutdown: ok",
	}

	for _, want := range expected {
		if !strings.Contains(output, want) {
			t.Fatalf("expected output to contain %q, got %q", want, output)
		}
	}
}
