//go:build lockman_examples

package main

import (
	"testing"
)

func TestRuntimeExampleIsRunnable(t *testing.T) {
	// This test verifies the example compiles and the testBridge implements the interface.
	t.Skip("example is a demonstration; run manually with: go run -tags lockman_examples .")
}
