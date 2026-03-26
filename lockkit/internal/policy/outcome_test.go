package policy

import (
	"errors"
	"testing"
)

func TestOutcomeFromErrorMapsDLQWrappedError(t *testing.T) {
	err := DLQ(errors.New("poison payload"))
	if got := OutcomeFromError(err); got != OutcomeDLQ {
		t.Fatalf("expected dlq outcome, got %q", got)
	}
}
