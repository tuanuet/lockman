package policy

import (
	"context"
	"errors"
	"fmt"

	lockerrors "lockman/lockkit/errors"
)

// WorkerOutcome normalizes worker callback and runtime errors for queue adapters.
type WorkerOutcome string

const (
	OutcomeAck   WorkerOutcome = "ack"
	OutcomeRetry WorkerOutcome = "retry"
	OutcomeDrop  WorkerOutcome = "drop"
	OutcomeDLQ   WorkerOutcome = "dlq"
)

var errDLQ = errors.New("worker dlq")

// DLQ wraps an error to force DLQ mapping through OutcomeFromError.
func DLQ(err error) error {
	if err == nil {
		return errDLQ
	}
	return fmt.Errorf("%w: %w", errDLQ, err)
}

// OutcomeFromError maps runtime and callback errors into normalized queue outcomes.
func OutcomeFromError(err error) WorkerOutcome {
	switch {
	case err == nil:
		return OutcomeAck
	case errors.Is(err, errDLQ):
		return OutcomeDLQ
	case errors.Is(err, lockerrors.ErrDuplicateIgnored):
		return OutcomeAck
	case errors.Is(err, lockerrors.ErrPolicyViolation):
		return OutcomeDrop
	case errors.Is(err, lockerrors.ErrInvariantRejected):
		return OutcomeDrop
	case errors.Is(err, lockerrors.ErrWorkerShuttingDown):
		return OutcomeRetry
	case errors.Is(err, lockerrors.ErrLockBusy):
		return OutcomeRetry
	case errors.Is(err, lockerrors.ErrOverlapRejected):
		return OutcomeRetry
	case errors.Is(err, lockerrors.ErrLockAcquireTimeout):
		return OutcomeRetry
	case errors.Is(err, lockerrors.ErrLeaseLost):
		return OutcomeRetry
	case errors.Is(err, context.Canceled):
		return OutcomeRetry
	case errors.Is(err, context.DeadlineExceeded):
		return OutcomeRetry
	default:
		return OutcomeRetry
	}
}
