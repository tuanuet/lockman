//go:build lockman_examples

package main

import (
	"bytes"
	"testing"
)

func TestMainPrintsInspectEndpointAndLifecycleSummary(t *testing.T) {
	// This test verifies the example output contains expected strings.
	// Since main() requires a running Redis, we skip the actual execution.
	t.Skip("example requires Redis; run manually with: go run -tags lockman_examples .")
}

func TestApproveOrderUseCaseIsDefined(t *testing.T) {
	if approveOrder.DefinitionID() != "order.approve" {
		t.Fatalf("expected definition ID %q, got %q", "order.approve", approveOrder.DefinitionID())
	}
}

func TestResourceIDBinding(t *testing.T) {
	req, err := approveOrder.With(approveInput{OrderID: "123"})
	if err != nil {
		t.Fatalf("With returned error: %v", err)
	}
	if req.ResourceKey != "order:123" {
		t.Fatalf("expected resource key %q, got %q", "order:123", req.ResourceKey)
	}
}

// Verify imports are used (compile-time check).
var _ = bytes.NewBuffer
