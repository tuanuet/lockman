package policy

import (
	"errors"
	"fmt"
	"testing"

	"lockman/backend"
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

func TestOutcomeFromErrorMapsBackendSentinels(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want WorkerOutcome
	}{
		{
			name: "invalid request is non-retriable",
			err:  backend.ErrInvalidRequest,
			want: OutcomeDrop,
		},
		{
			name: "invalid request wrapped stays non-retriable",
			err:  fmt.Errorf("wrapped: %w", backend.ErrInvalidRequest),
			want: OutcomeDrop,
		},
		{
			name: "lease already held is retriable contention",
			err:  backend.ErrLeaseAlreadyHeld,
			want: OutcomeRetry,
		},
		{
			name: "lease not found is retriable",
			err:  backend.ErrLeaseNotFound,
			want: OutcomeRetry,
		},
		{
			name: "lease expired is retriable",
			err:  backend.ErrLeaseExpired,
			want: OutcomeRetry,
		},
		{
			name: "lease owner mismatch is retriable",
			err:  backend.ErrLeaseOwnerMismatch,
			want: OutcomeRetry,
		},
		{
			name: "joined invalid request stays non-retriable",
			err:  errors.Join(errors.New("other"), backend.ErrInvalidRequest),
			want: OutcomeDrop,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := OutcomeFromError(tc.err)
			if got != tc.want {
				t.Fatalf("OutcomeFromError(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}
