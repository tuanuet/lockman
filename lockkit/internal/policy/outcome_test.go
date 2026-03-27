package policy

import (
	"errors"
	"fmt"
	"testing"

	lockerrors "lockman/lockkit/errors"
)

func TestOutcomeFromErrorMapsDLQWrappedError(t *testing.T) {
	err := DLQ(errors.New("poison payload"))
	if got := OutcomeFromError(err); got != OutcomeDLQ {
		t.Fatalf("expected dlq outcome, got %q", got)
	}
}

func TestOutcomeFromErrorTreatsOverlapAsRetry(t *testing.T) {
	if got := OutcomeFromError(lockerrors.ErrOverlapRejected); got != OutcomeRetry {
		t.Fatalf("expected retry, got %q", got)
	}
}

func TestOutcomeFromErrorTreatsWrappedOverlapAsRetry(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", lockerrors.ErrOverlapRejected)
	if got := OutcomeFromError(err); got != OutcomeRetry {
		t.Fatalf("expected retry, got %q", got)
	}
}
