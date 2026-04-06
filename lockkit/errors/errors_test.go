package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrReentrantAcquireMatchesWithErrorsIs(t *testing.T) {
	err := fmt.Errorf("runtime rejected acquire: %w", ErrReentrantAcquire)
	if !errors.Is(err, ErrReentrantAcquire) {
		t.Fatal("expected ErrReentrantAcquire to match with errors.Is")
	}
}

func TestPhase2ErrorsMatchWithErrorsIs(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "duplicate ignored", err: ErrDuplicateIgnored},
		{name: "invariant rejected", err: ErrInvariantRejected},
		{name: "worker shutting down", err: ErrWorkerShuttingDown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("phase2 error: %w", tc.err)
			if !errors.Is(wrapped, tc.err) {
				t.Fatalf("expected %q to match with errors.Is", tc.name)
			}
		})
	}
}
