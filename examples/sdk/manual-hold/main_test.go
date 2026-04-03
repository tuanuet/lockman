//go:build lockman_examples

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func TestManualHoldUseCaseIsDefined(t *testing.T) {
	if manualHold.DefinitionID() != "order.manual_hold" {
		t.Fatalf("expected hold use case DefinitionID %q, got %q", "order.manual_hold", manualHold.DefinitionID())
	}

	req, err := manualHold.With(holdInput{OrderID: "123"})
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	if req.ResourceKey() != "order:123" {
		t.Fatalf("expected resource key %q, got %q", "order:123", req.ResourceKey())
	}
}

func TestManualHoldOutput(t *testing.T) {
	redisServer, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis run failed: %v", err)
	}
	defer redisServer.Close()

	var out bytes.Buffer
	client := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	defer client.Close()

	if err := run(&out, client); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "hold resource key: order:123") {
		t.Fatalf("unexpected output: %s", output)
	}
	if !strings.Contains(output, "hold token: h1_") {
		t.Fatalf("unexpected output: %s", output)
	}
	if !strings.Contains(output, "forfeit: ok") {
		t.Fatalf("unexpected output: %s", output)
	}
	if !strings.Contains(output, "shutdown: ok") {
		t.Fatalf("unexpected output: %s", output)
	}
}
